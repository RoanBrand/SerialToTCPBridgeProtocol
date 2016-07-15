package protocol

import (
	"hash/crc32"
	"encoding/binary"
)

const (
	publish = iota
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
