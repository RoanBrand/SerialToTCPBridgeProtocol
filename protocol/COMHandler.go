package protocol

import (
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

	acknowledgeEvent chan bool
	startEvent       chan bool
	errorEvent       chan<- bool

	expectedRxSeqFlag bool
}

func NewComHandler(ComName string, ComBaudrate int, exitSignal chan<- bool) (*comHandler, error) {
	newListener := comHandler{
		state:             disconnected,
		rxBuffer:          make(chan byte, 512),
		txBuffer:          make(chan Packet, 2),
		acknowledgeEvent:  make(chan bool),
		startEvent:        make(chan bool),
		errorEvent:        exitSignal,
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
	if com.tcpLink != nil {
		com.tcpLink.Close()
	}
	com.comPort.Close()
}

// Wait for client to initiate connection
func (com *comHandler) listenAndServeClient() {
	go com.rxCOM()
	go com.packetReader()
	log.Println("started waiting for connect packet")
	_ = <-com.startEvent

	go com.txCOM()
	go com.packetSender()
}

// Receive from Serial COM interface and send through buffered channel.
// Separate Goroutine decouples rx and parsing of packets.
func (com *comHandler) rxCOM() {
	rx := make([]byte, 128)
	com.comPort.Flush()
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
	for txPacket := range com.txBuffer {
		txPacket.length = byte(len(txPacket.payload) + 5)
		txPacket.crc = txPacket.calcCrc()
		serialPacket := txPacket.serialize()

		nTx, err := com.comPort.Write(serialPacket)
		if err != nil {
			log.Fatal(err)
			//continue - future better error handling
		}
		if nTx != len(serialPacket) {
			log.Printf("COM Send mismatch. Want to send %v bytes. Sent: %v bytes.", len(serialPacket), nTx)
		}
	}
}

func (com *comHandler) dialTCP(destination string) (err error) {
	com.tcpLink, err = net.Dial("tcp", destination)
	return
}
