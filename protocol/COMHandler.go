package protocol

import (
	"errors"
	"github.com/tarm/serial"
	"log"
	"net"
	"sync"
)

// Connection State
const (
	COMNotEstablished = iota
	disconnected
	connected
)

type comHandler struct {
	upstreamLink net.Conn
	comPort      *serial.Port
	comConfig    *serial.Config

	state uint8

	rxBuffer chan byte
	txBuffer chan Packet

	acknowledgeEvent  chan bool
	expectedRxSeqFlag bool
	session           sync.WaitGroup
}

func NewComHandler(ComName string, ComBaudRate int) *comHandler {
	new := comHandler{
		comConfig: &serial.Config{Name: ComName, Baud: ComBaudRate},
		state:     COMNotEstablished,
	}
	return &new
}

// Service a protocol client on a single COM interface
func (com *comHandler) ListenAndServeClient() error {
	var err error
	com.comPort, err = serial.OpenPort(com.comConfig)
	if err != nil {
		return err
	}

	com.rxBuffer = make(chan byte, 512)
	com.acknowledgeEvent = make(chan bool)
	com.state = disconnected

	log.Println("Listening on", com.comConfig.Name)

	// Start COM RX
	com.session.Add(2)
	go com.rxCOM()
	go com.packetReader()

	com.session.Wait()
	return errors.New("COM Port Lost")
}

// Receive from Serial COM interface and send through buffered channel.
// Separate Goroutine decouples rx and parsing of packets.
func (com *comHandler) rxCOM() {
	defer com.session.Done()
	rx := make([]byte, 128)
	com.comPort.Flush()
	for {
		nRx, err := com.comPort.Read(rx)
		if err != nil {
			log.Printf("Error reading from COM: %v\n", err)
			com.dropCOM()
			return
		}
		for _, v := range rx[:nRx] {
			com.rxBuffer <- v
		}
	}
}

// Receive from channel and write to COM.
func (com *comHandler) txCOM() {
	defer com.session.Done()
	for txPacket := range com.txBuffer {
		txPacket.length = byte(len(txPacket.payload) + 5)
		txPacket.crc = txPacket.calcCrc()
		serialPacket := txPacket.serialize()

		nTx, err := com.comPort.Write(serialPacket)
		if err != nil {
			log.Printf("Error writing to COM: %v\n", err)
			com.dropCOM()
			return
		}

		if nTx != len(serialPacket) {
			log.Printf("COM TX mismatch. Want to send %v bytes. Sent: %v bytes.", len(serialPacket), nTx)
		}
	}
}

// Open connection to upstream server on behalf of client
func (com *comHandler) dialUpstream(destination string) (err error) {
	com.upstreamLink, err = net.Dial("tcp", destination)
	return
}

// Stop activity and release COM interface
func (com *comHandler) dropCOM() {
	com.dropLink()
	com.comPort.Close()
	close(com.rxBuffer)
	com.state = COMNotEstablished
}
