package protocol

import (
	"encoding/binary"
	"hash/crc32"
	"log"
	"strconv"
	"time"
)

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
PACKET_RX_LOOP:
	for {
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
			continue PACKET_RX_LOOP // discard
		}

		// Payload
		var payloadByte byte
		for i := 0; i < int(p.length)-5; i++ {
			select {
			case payloadByte, ok = <-com.rxBuffer:
				if !ok {
					return
				}
				p.payload = append(p.payload, payloadByte)
			case <-time.After(time.Millisecond * 100):
				log.Println("<<<Packet in from COM TIMEOUT")
				continue PACKET_RX_LOOP
			}
		}

		// CRC32
		rxCrc := make([]byte, 0, 4)
		var crcByte byte
		for i := 0; i < 4; i++ {
			select {
			case crcByte, ok = <-com.rxBuffer:
				if !ok {
					return
				}
				rxCrc = append(rxCrc, crcByte)
			case <-time.After(time.Millisecond * 100):
				log.Println("<<<Packet in from COM TIMEOUT")
				continue PACKET_RX_LOOP
			}
		}
		p.crc = binary.LittleEndian.Uint32(rxCrc)

		// Integrity Checking
		if p.calcCrc() != p.crc {
			log.Println("<<<Packet in from COM CRCFAIL")
			continue PACKET_RX_LOOP
		}

		// Packet receive done. Process it.
		com.handleRxPacket(&p)
	}
}

func (com *comHandler) handleRxPacket(packet *Packet) {
	rxSeqFlag := (packet.command & 0x80) > 0
	switch packet.command & 0x7F {
	case publish:
		// Payload from serial client
		if com.state == connected {
			com.txBuffer <- Packet{command: acknowledge | (packet.command & 0x80)}
			if rxSeqFlag == com.expectedRxSeqFlag {
				com.expectedRxSeqFlag = !com.expectedRxSeqFlag
				com.tcpLink.Write(packet.payload)
			}
		}
	case acknowledge:
		com.acknowledgeEvent <- rxSeqFlag // TODO: if not connected this will block forever
	case connect:
		log.Println("got CONNECT PACKET")
		if com.state != disconnected {
			return
		}
		if len(packet.payload) != 6 {
			return
		}
		port := binary.LittleEndian.Uint16(packet.payload[4:])
		destination := strconv.Itoa(int(packet.payload[0])) + "." + strconv.Itoa(int(packet.payload[1])) + "." + strconv.Itoa(int(packet.payload[2])) + "." + strconv.Itoa(int(packet.payload[3])) + ":" + strconv.Itoa(int(port))
		log.Printf("Dialing to: %v", destination)
		if err := com.dialTCP(destination); err != nil { // TODO: add timeout
			com.txBuffer <- Packet{command: disconnect} // TODO: payload to contain error or timeout
			return
		}
		com.startEvent <- true
		com.state = connected
		com.txBuffer <- Packet{command: connack}
	}
}

// Publish packet received from a channel.
// Will block for second publish, until ack is received for first.
func (com *comHandler) packetSender() {
	sequenceTxFlag := false
	tx := make([]byte, 512)
	for {
		nRx, err := com.tcpLink.Read(tx)
		if err != nil {
			log.Fatal("Error Receiving from TCP")
		}
		log.Println(">>>Packet out to COM START")
		log.Println("------------------->>>>>>>")
		p := Packet{command: publish, payload: tx[:nRx]}
		if sequenceTxFlag {
			p.command |= 0x80
		}
		for {
			com.txBuffer <- p
			ack := <-com.acknowledgeEvent
			if ack == sequenceTxFlag {
				sequenceTxFlag = !sequenceTxFlag
				break
			}
			log.Println(">>>RETRY out to COM")
		}
		log.Println(">>>Packet out to COM DONE")
	}
}
