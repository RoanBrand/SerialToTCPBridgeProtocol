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
	for {
		p := Packet{}

		// Length byte
	WAIT_FOR_FIRST_BYTE:
		for {
			select {
			case p.length = <-com.rxBuffer:
				break WAIT_FOR_FIRST_BYTE
			default:
				// Loop until we get the first byte
			}
		}
		log.Println("<<<Packet in from COM START")
		log.Println("<<<<<-------------------")

		// Command byte
		select {
		case p.command = <-com.rxBuffer:
		// Received
		case <-time.After(time.Second):
			continue // discard
		}

		// Payload
		pLen := int(p.length)
		timeout := false
		for i := 0; i < pLen-5; i++ {
			select {
			case payloadByte := <-com.rxBuffer:
				p.payload = append(p.payload, payloadByte)
			case <-time.After(time.Second):
				timeout = true
			}
			if timeout {
				break
			}
		}
		if timeout {
			log.Println("<<<Packet in from COM TIMEOUT")
			continue // discard
		}

		// CRC32
		rxCrc := make([]byte, 0, 4)
		for i := 0; i < 4; i++ {
			select {
			case crcByte := <-com.rxBuffer:
				rxCrc = append(rxCrc, crcByte)
			case <-time.After(time.Second):
				timeout = true
			}
			if timeout {
				break
			}
		}
		if timeout {
			log.Println("<<<Packet in from COM TIMEOUT")
			continue // discard
		}
		p.crc = binary.LittleEndian.Uint32(rxCrc)

		// Integrity Checking
		if p.calcCrc() != p.crc {
			log.Println("<<<Packet in from COM CRCFAIL")
			continue // discard
		}

		// Packet receive done. Process it.
		log.Println("<<<Packet in from COM DONE")
		rxSeqFlag := (p.command & 0x80) > 0
		switch p.command & 0x7F {
		case publish:
			// STM32 sent us a payload
			com.txBuffer <- Packet{command: acknowledge | (p.command & 0x80)}
			if rxSeqFlag == com.expectedRxSeqFlag {
				com.expectedRxSeqFlag = !com.expectedRxSeqFlag
				com.tcpLink.Write(p.payload)
			}
		case acknowledge:
			com.acknowledgeChan <- rxSeqFlag
		case connect:
			log.Println("got CONNECT PACKET")
			if com.state != disconnected {
				continue
			}
			if len(p.payload) != 6 {
				continue
			}
			port := binary.LittleEndian.Uint16(p.payload[4:])
			destination := strconv.Itoa(int(p.payload[0])) + "." + strconv.Itoa(int(p.payload[1])) + "." + strconv.Itoa(int(p.payload[2])) + "." + strconv.Itoa(int(p.payload[3])) + ":" + strconv.Itoa(int(port))
			log.Printf("Dialing to: %v", destination)
			if err := com.dialTCP(destination); err != nil {
				com.txBuffer <- Packet{command: disconnect}
				continue
			}
			com.state = connected
			com.txBuffer <- Packet{command: connack}
		}
	}
}

// Publish packet received from a channel.
// Will block for second publish, until ack is received for first.
func (com *comHandler) packetSender() {
	sequenceTxFlag := false
	for {
		p := <-com.comSend
		log.Println(">>>Packet out to COM START")
		log.Println("------------------->>>>>>>")
		if sequenceTxFlag {
			p.command |= 0x80
		} else {
			p.command &= 0x7F // may not be necessary if seq never set
		}
		for {
			com.txBuffer <- p
			ack := <-com.acknowledgeChan
			if ack == sequenceTxFlag {
				sequenceTxFlag = !sequenceTxFlag
				break
			}
			log.Println(">>>RETRY out to COM")
		}
		log.Println(">>>Packet out to COM DONE")
	}
}

func (com *comHandler) tcpReader() {
	tx := make([]byte, 128)
	for {
		nRx, err := com.tcpLink.Read(tx)
		if err != nil {
			log.Fatal("Error Receiving from TCP")
		}
		com.comSend <- Packet{command: publish, payload: tx[:nRx]}
	}
}
