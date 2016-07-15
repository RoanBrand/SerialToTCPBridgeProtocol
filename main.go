package main

import (
	"encoding/binary"
	"fmt"
	"github.com/tarm/serial"
	"hash/crc32"
	"log"
	"net"
	"time"
)

const (
	publish = iota
	acknowledge
)

type packet struct {
	length  byte
	command byte
	payload []byte
	crc     uint32
}

func (p packet) serialize() []byte {
	ser := make([]byte, 0, 8)
	ser = append(ser, p.length)
	ser = append(ser, p.command)
	ser = append(ser, p.payload...)
	crcBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(crcBytes, p.crc)
	ser = append(ser, crcBytes...)

	return ser
}

func (p packet) calcCrc() uint32 {
	return crc32.ChecksumIEEE(p.serialize()[:len(p.payload)+2])
}

func main() {
	config := &serial.Config{Name: "COM6", Baud: 115200}
	port, err := serial.OpenPort(config)
	if err != nil {
		log.Fatal("Error opening COM Port")
	}
	defer port.Close()
	tcpCon, err := net.Dial("tcp", "127.0.0.1:1883")
	if err != nil {
		log.Fatal("Error opening TCP Port")
	}
	defer tcpCon.Close()

	rxBuffer := make(chan byte, 512)
	c_send := make(chan packet)
	c_ack := make(chan bool)

	sendCOM := make(chan packet, 2)
	receiveCOM := make(chan packet)

	// Receive from Serial COM interface and send through buffered channel.
	// Separate Goroutine decouples rx and parsing of packets.
	rxCOM := func() {
		rx := make([]byte, 128)
		for {
			nRx, err := port.Read(rx)
			if err != nil {
				log.Fatal(err)
			}
			for _, v := range rx[:nRx] {
				rxBuffer <- v
			}
		}
	}
	go rxCOM()

	// Receive from channel and write to COM.
	txCOM := func() {
		for {
			txPacket := <-sendCOM

			txPacket.length = uint8(len(txPacket.payload) + 5)
			txPacket.crc = txPacket.calcCrc()
			serialPacket := txPacket.serialize()

			nTx, err := port.Write(serialPacket)
			if err != nil {
				log.Fatal(err)
				//continue - future better error handling
			}
			if nTx != len(serialPacket) {
				fmt.Println("ERROR!!")
				fmt.Println("Need to send over uart: ", len(serialPacket))
				fmt.Println("Sent over uart: ", nTx)
			}
		}
	}
	go txCOM()

	// Publish packet received from a channel.
	// Will block for second publish, until ack is received for first.
	packetSender := func() {
		sequenceTxFlag := false
		for {
			p := <-c_send
			log.Println(">>>Packet out to COM START")
			log.Println("------------------->>>>>>>")
			if sequenceTxFlag {
				p.command |= 0x80
			} else {
				p.command &= 0x7F // may not be necessary if seq never set
			}
			for {
				sendCOM <- p
				ack := <-c_ack
				if ack == sequenceTxFlag {
					sequenceTxFlag = !sequenceTxFlag
					break
				}
				log.Println(">>>RETRY out to COM")
			}
			log.Println(">>>Packet out to COM DONE")
		}
	}
	go packetSender()

	// Parse receive buffered channel for legitimate packets.
	packetReader := func() {
		for {
			p := packet{}

			// Length byte
		WAIT:
			for {
				select {
				case p.length = <-rxBuffer:
					break WAIT
				default:
					// Loop until we get a first byte
				}
			}
			log.Println("<<<Packet in from COM START")
			log.Println("<<<<<-------------------")

			// Command byte
			select {
			case p.command = <-rxBuffer:
				// Received
			case <-time.After(time.Second):
				continue // discard
			}

			// Payload
			pLen := int(p.length)
			timeout := false
			for i := 0; i < pLen-5; i++ {
				select {
				case payloadByte := <-rxBuffer:
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
				case crcByte := <-rxBuffer:
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

			receiveCOM <- p

			log.Println("<<<Packet in from COM DONE")
		}
	}
	go packetReader()

	// Parse received packets ( MAke this in same goroutine as packetReader. Separate function
	handleRxPackets := func() {
		expectedRxSeqFlag := false
		for {
			p := <-receiveCOM
			rxSeqFlag := (p.command & 0x80) > 0
			switch p.command & 0x7F {
			case publish:
				// STM32 sent us a payload
				sendCOM <- packet{command: acknowledge | (p.command & 0x80)}
				if rxSeqFlag == expectedRxSeqFlag {
					expectedRxSeqFlag = !expectedRxSeqFlag
					tcpCon.Write(p.payload)
				}
			case acknowledge:
				c_ack <- rxSeqFlag
			}
		}
	}
	go handleRxPackets()
	log.Println("Starting Server")
	tx := make([]byte, 128)
	for {
		nRx, err := tcpCon.Read(tx)
		if err != nil {
			log.Fatal("Error Receiving from TCP")
		}
		c_send <- packet{command: publish, payload: tx[:nRx]}
	}

}
