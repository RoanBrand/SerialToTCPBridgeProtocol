package protocol

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"log"
	"strconv"
	"time"
)

// Protocol Commands
const (
	connect = iota
	connack
	disconnect
	publish
	acknowledge
)

type Packet struct {
	length  byte
	command byte
	payload []byte
	crc     uint32
}

func (p Packet) serialize() []byte {
	ser := make([]byte, 0, 8)
	ser = append(ser, p.length)
	ser = append(ser, p.command)
	ser = append(ser, p.payload...)
	crcBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(crcBytes, p.crc)
	ser = append(ser, crcBytes...)
	return ser
}

func (p Packet) calcCrc() uint32 {
	return crc32.ChecksumIEEE(p.serialize()[:len(p.payload)+2])
}

// Parse receive buffered channel for legitimate packets.
func (com *comHandler) packetReader() {
	defer com.session.Done()
	timeouts := 0
PACKET_RX_LOOP:
	for {
		if timeouts >= 5 {
			if com.state == connected {
				log.Println("COM RX too many timeouts. Disconnecting client")
				com.txBuffer <- Packet{command: disconnect}
				com.dropLink()
				return
			}
			timeouts = 0
		}

		p := Packet{}
		var ok bool

		// Length byte
		p.length, ok = <-com.rxBuffer
		if !ok {
			return
		}

		// Command byte
		select {
		case p.command, ok = <-com.rxBuffer:
			if !ok {
				return
			}
		case <-time.After(time.Millisecond * 100):
			timeouts++
			continue PACKET_RX_LOOP // discard
		}

		// Payload
		for i := 0; i < int(p.length)-5; i++ {
			select {
			case payloadByte, ok := <-com.rxBuffer:
				if !ok {
					return
				}
				p.payload = append(p.payload, payloadByte)
			case <-time.After(time.Millisecond * 100):
				timeouts++
				continue PACKET_RX_LOOP
			}
		}

		// CRC32
		rxCrc := make([]byte, 0, 4)
		for i := 0; i < 4; i++ {
			select {
			case crcByte, ok := <-com.rxBuffer:
				if !ok {
					return
				}
				rxCrc = append(rxCrc, crcByte)
			case <-time.After(time.Millisecond * 100):
				timeouts++
				continue PACKET_RX_LOOP
			}
		}
		p.crc = binary.LittleEndian.Uint32(rxCrc)

		// Integrity Checking
		if p.calcCrc() != p.crc {
			log.Println("Packet RX COM CRCFAIL")
			timeouts++
			continue PACKET_RX_LOOP
		}
		timeouts = 0
		com.handleRxPacket(&p)
	}
}

// Packet RX done. Handle it.
func (com *comHandler) handleRxPacket(packet *Packet) {
	var rxSeqFlag bool = (packet.command & 0x80) > 0
	switch packet.command & 0x7F {
	case publish:
		// Payload from serial client
		if com.state == connected {
			com.txBuffer <- Packet{command: acknowledge | (packet.command & 0x80)}
			if rxSeqFlag == com.expectedRxSeqFlag {
				com.expectedRxSeqFlag = !com.expectedRxSeqFlag
				_, err := com.upstreamLink.Write(packet.payload)
				if err != nil {
					log.Printf("Error sending upstream: %v Disconnecting client\n", err)
					com.txBuffer <- Packet{command: disconnect}
					com.dropLink()
				}
			}
		}
	case acknowledge:
		if com.state == connected {
			com.acknowledgeEvent <- rxSeqFlag
		}
	case connect:
		if com.state != disconnected {
			return
		}
		if len(packet.payload) != 6 {
			return
		}

		port := binary.LittleEndian.Uint16(packet.payload[4:])

		var dst bytes.Buffer
		for i := 0; i < 3; i++ {
			dst.WriteString(strconv.Itoa(int(packet.payload[i])))
			dst.WriteByte('.')
		}
		dst.WriteString(strconv.Itoa(int(packet.payload[3])))
		dst.WriteByte(':')
		dst.WriteString(strconv.Itoa(int(port)))
		dstStr := dst.String()

		com.txBuffer = make(chan Packet, 2)
		com.expectedRxSeqFlag = false

		// Start COM TX
		com.session.Add(1)
		go com.txCOM()

		log.Printf("Dialing to: %v\n", dstStr)
		if err := com.dialUpstream(dstStr); err != nil { // TODO: add timeout
			log.Printf("Failed to connect to: %v\n", dstStr)
			com.txBuffer <- Packet{command: disconnect} // TODO: payload to contain error or timeout
			com.dropLink()
			return
		}

		// Start link session
		com.session.Add(1)
		go com.packetSender()
		log.Printf("Connected to %v\n", dstStr)
		com.state = connected
		com.txBuffer <- Packet{command: connack}
	case disconnect:
		if com.state == connected {
			log.Println("Client wants to disconnect. Ending link session")
			com.dropLink()
		}
	}
}

// Publish data received from upstream tcp server.
// We need to get an Ack before sending the next publish packet.
// Resend same publish packet after timeout, and kill link after 5 retries.
func (com *comHandler) packetSender() {
	defer com.session.Done()
	sequenceTxFlag := false
	retries := 0
	tx := make([]byte, 512)
	for {
		nRx, err := com.upstreamLink.Read(tx)
		if err != nil {
			if com.state == connected {
				log.Printf("Error receiving upstream: %v. Disconnecting client\n", err)
				com.txBuffer <- Packet{command: disconnect}
				com.dropLink()
			}
			return
		}
		p := Packet{command: publish, payload: tx[:nRx]}
		if sequenceTxFlag {
			p.command |= 0x80
		}
	PUB_LOOP:
		for {
			com.txBuffer <- p
			select {
			case ack, ok := <-com.acknowledgeEvent:
				if ok && ack == sequenceTxFlag {
					retries = 0
					sequenceTxFlag = !sequenceTxFlag
					break PUB_LOOP // success
				}
			case <-time.After(time.Millisecond * 500):
				retries++
				if retries >= 5 {
					log.Println("Too many downstream send retries. Disconnecting client")
					com.txBuffer <- Packet{command: disconnect}
					com.dropLink()
					return
				}
			}
		}
	}
}

// End link session between upstream server and client
func (com *comHandler) dropLink() {
	if com.upstreamLink != nil {
		com.upstreamLink.Close()
	}
	if com.txBuffer != nil {
		close(com.txBuffer)
		com.txBuffer = nil
	}
	com.state = disconnected
}
