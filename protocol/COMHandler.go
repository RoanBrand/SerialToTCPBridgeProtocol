package protocol

import (
	"fmt"
	"github.com/tarm/serial"
	"log"
	"net"
)

const (
	disconnected = iota
	connected
)

type comHandler struct {
	tcpLink net.Conn
	comPort *serial.Port

	state uint8

	rxBuffer chan byte
	txBuffer chan Packet

	acknowledgeChan chan bool
	expectedRxSeqFlag bool
}

func NewComHandler(ComName string, ComBaudrate int) (*comHandler, error) {
	newListener := comHandler{
		state:             disconnected,
		rxBuffer:          make(chan byte, 512),
		txBuffer:          make(chan Packet, 2),
		acknowledgeChan:   make(chan bool),
		expectedRxSeqFlag: false,
	}
	var err error
	config := &serial.Config{Name: ComName, Baud: ComBaudrate}
	newListener.comPort, err = serial.OpenPort(config)
	if err != nil {
		return nil, err
	}
	go newListener.listenAndServeClient()
	return &newListener, nil
}

func (com *comHandler) EndGracefully() {
	com.tcpLink.Close()
	com.comPort.Close()
}

// Wait for client to initiate connection
func (com *comHandler) listenAndServeClient() {
	go com.rxCOM()
	go com.packetReader()
	log.Println("started waiting for connect packet")
	for com.state != connected {
	}

	go com.txCOM()
	go com.packetSender()
}

// Receive from Serial COM interface and send through buffered channel.
// Separate Goroutine decouples rx and parsing of packets.
func (com *comHandler) rxCOM() {
	rx := make([]byte, 128)
	for {
		nRx, err := com.comPort.Read(rx)
		if err != nil {
			log.Printf("error reading from com port. device gone? -> %v\n", err)
			close(com.rxBuffer)
			return
		}
		for _, v := range rx[:nRx] {
			com.rxBuffer <- v
		}
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

func (com *comHandler) dialTCP(destination string) (err error) {
	com.tcpLink, err = net.Dial("tcp", destination)
	return
}
