package protocol

import (
	"encoding/binary"
	"hash/crc32"
	"log"
	"strconv"
	"time"
)

// Protocol commands.
const (
	connect = iota
	connack
	disconnect
	publish
	acknowledge
)

// Protocol packet and helpers.
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

// Parse RX buffer for legitimate packets.
func (t *protocolTransport) packetParser(packetHandler func(*Packet), onTimeout func()) {
	defer t.session.Done()
	timeouts := 0
PACKET_RX_LOOP:
	for {
		if timeouts >= 5 {
			if t.state == Connected {
				log.Println("RX packet timeout")
				t.txBuff <- Packet{command: disconnect}
				if onTimeout != nil {
					onTimeout()
				}
				return
			}
			timeouts = 0
		}

		p := Packet{}
		var ok bool

		// Length byte
		p.length, ok = <-t.rxBuff
		if !ok {
			return
		}

		// Command byte
		select {
		case p.command, ok = <-t.rxBuff:
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
			case payloadByte, ok := <-t.rxBuff:
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
			case crcByte, ok := <-t.rxBuff:
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
			log.Println("RX packet CRCFAIL")
			timeouts++
			continue PACKET_RX_LOOP
		}
		timeouts = 0
		packetHandler(&p)
	}
}

// Publish data over Serial interface.
// We need to get an Ack before sending the next publish packet.
// Resend same publish packet after timeout, and kill link after 5 retries.
func (t *protocolTransport) packetSender(getData func() (Packet, error), onError func()) {
	defer t.session.Done()
	sequenceTxFlag := false
	retries := 0
	for {
		p, err := getData()
		if err != nil {
			if t.state == Connected {
				log.Printf("Error receiving data: %v. Disconnecting from Protocol partner\n", err)
				t.txBuff <- Packet{command: disconnect}
				if onError != nil {
					onError()
				}
			}
			return
		}
		if sequenceTxFlag {
			p.command |= 0x80
		}
	PUB_LOOP:
		for {
			t.txBuff <- p
			select {
			case ack, ok := <-t.acknowledgeEvent:
				if ok && ack == sequenceTxFlag {
					retries = 0
					sequenceTxFlag = !sequenceTxFlag
					break PUB_LOOP // success
				}
			case <-time.After(time.Millisecond * 500):
				retries++
				if retries >= 5 {
					log.Println("Too many tx serial retries. Disconnecting from Protocol partner")
					t.txBuff <- Packet{command: disconnect}
					if onError != nil {
						onError()
					}
					return
				}
			}
		}
	}
}

// Generate tcp connection string used to dial tcp server from Protocol Client's connect packet payload.
func makeTCPConnString(connPayload []byte) string {
	port := binary.LittleEndian.Uint16(connPayload[4:])
	connString := ""
	for i := 0; i < 3; i++ {
		connString += strconv.Itoa(int(connPayload[i])) + "."
	}
	connString += strconv.Itoa(int(connPayload[3])) + ":" + strconv.Itoa(int(port))
	return connString
}
