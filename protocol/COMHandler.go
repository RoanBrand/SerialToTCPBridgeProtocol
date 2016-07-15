package protocol

import (
	"encoding/binary"
	"fmt"
	"github.com/tarm/serial"
	"log"
	"time"
	"net"
)

type comHandler struct {
	comPort           *serial.Port
	rxBuffer          chan byte
	txBuffer          chan Packet

	acknowledgeChan   chan bool
	comSend           chan Packet

	tcpLink           net.Conn

	expectedRxSeqFlag bool
}

func NewComHandler(tcp *net.Conn) (*comHandler, error) {
	var newListener comHandler
	config := &serial.Config{Name: "COM6", Baud: 115200}
	port, err := serial.OpenPort(config)
	if err != nil {
		return nil, err
	}
	newListener.comPort = port
	newListener.rxBuffer = make(chan byte, 512)
	newListener.txBuffer = make(chan Packet, 2)

	newListener.acknowledgeChan = make(chan bool)
	newListener.comSend = make(chan Packet)

	newListener.tcpLink = *tcp
	newListener.expectedRxSeqFlag = false

	go newListener.rxCOM()
	go newListener.txCOM()
	go newListener.packetReader()
	go newListener.packetSender()
	go newListener.tcpReader()

	return &newListener, nil
}

func (com *comHandler) EndGracefully() {
	com.comPort.Close()
}

// Receive from Serial COM interface and send through buffered channel.
// Separate Goroutine decouples rx and parsing of packets.
func (com *comHandler) rxCOM() {
	rx := make([]byte, 128)
	for {
		nRx, err := com.comPort.Read(rx)
		if err != nil {
			log.Fatal(err)
		}
		for _, v := range rx[:nRx] {
			com.rxBuffer <- v
		}
	}
}

// Parse receive buffered channel for legitimate packets.
func (com *comHandler) packetReader() {
	for {
		p := Packet{}

		// Length byte
	WAIT:
		for {
			select {
			case p.length = <-com.rxBuffer:
				break WAIT
			default:
				// Loop until we get a first byte
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
		}

		log.Println("<<<Packet in from COM DONE")
	}
}

// Receive from channel and write to COM.
func (com *comHandler) txCOM() {
	for {
		txPacket := <-com.txBuffer

		txPacket.length = uint8(len(txPacket.payload) + 5)
		txPacket.crc = txPacket.calcCrc()
		serialPacket := txPacket.serialize()

		nTx, err := com.comPort.Write(serialPacket)
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
