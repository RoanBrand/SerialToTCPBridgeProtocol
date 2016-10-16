package protocol

import (
	"encoding/binary"
	"hash/crc32"
	"sync"
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

// Serial connection over which protocol runs.
// Represents simple 2-way noisy wire.
// Typically, a RS-232 Port or UART would implement this interface.
type protocolTransport interface {
	Read(p []byte) (n int, err error)
	Write(p []byte) (n int, err error)
	Close() error
	Flush() error
}

// Session link control.
type protocolSession struct {
	state             uint8
	session           sync.WaitGroup
	rxBuff            chan byte
	txBuff            chan Packet
	acknowledgeEvent  chan bool
	expectedRxSeqFlag bool
}

// Protocol session connection states.
const (
	TransportNotReady = iota
	Disconnected
	Connected
)

// Used for debug logging
var logLock sync.RWMutex
