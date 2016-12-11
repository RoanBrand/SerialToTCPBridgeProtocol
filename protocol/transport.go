package protocol

import (
	"log"
	"sync"
)

// Used for debug logging
var logLock sync.RWMutex

// Serial connection over which protocol runs.
// Represents simple 2-way noisy wire.
// Typically, a RS-232 Port or UART would implement this interface.
type serialInterface interface {
	Read(p []byte) (n int, err error)
	Write(p []byte) (n int, err error)
	Close() error
	Flush() error
}

// Protocol session connection states.
const (
	TransportNotReady = iota
	Disconnected
	Connected
)

// Transport/Session control between two Protocol entities.
type protocolTransport struct {
	state             uint8
	session           sync.WaitGroup
	com               serialInterface
	rxBuff            chan byte
	txBuff            chan Packet
	acknowledgeEvent  chan bool
	expectedRxSeqFlag bool
}

// Receive from serial wire and write to buffer.
func (t *protocolTransport) rxSerial(onReadFail func()) {
	defer t.session.Done()
	rx := make([]byte, 512)
	t.com.Flush()
	for {
		nRx, err := t.com.Read(rx)
		if err != nil {
			log.Printf("Error receiving on COM: %v\n", err)
			if onReadFail != nil {
				onReadFail()
			}
			return
		}
		/*
			// For debugging
			logLock.Lock()
			log.Println("serial RX:")
			log.Println(rx[:nRx])
			log.Println(string(rx[:nRx]))
			logLock.Unlock()
		*/
		for _, v := range rx[:nRx] {
			t.rxBuff <- v
		}
	}
}

// Read from TX buffer and write out downstream.
func (t *protocolTransport) txSerial(onWriteFail func()) {
	defer t.session.Done()
	for txPacket := range t.txBuff {
		txPacket.length = byte(len(txPacket.payload) + 5)
		txPacket.crc = txPacket.calcCrc()
		serialPacket := txPacket.serialize()

		nTx, err := t.com.Write(serialPacket)
		if err != nil {
			log.Printf("Error writing to COM: %v\n", err)
			if onWriteFail != nil {
				onWriteFail()
			}
			return
		}
		/*
			// For debugging
			logLock.Lock()
			log.Println("serial TX:")
			log.Println(serialPacket)
			log.Println(string(serialPacket[2:]))
			logLock.Unlock()
		*/
		if nTx != len(serialPacket) {
			log.Printf("TX mismatch. Want to send %v bytes. Sent: %v bytes.", len(serialPacket), nTx)
		}
	}
}
