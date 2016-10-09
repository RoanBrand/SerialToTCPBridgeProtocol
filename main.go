package main

import (
	"github.com/RoanBrand/SerialToTCPBridgeProtocol/protocol"
	"log"
	"time"
)

const (
	COMPortName = "COM3"
	COMBaudRate = 115200
)

func main() {
	com := protocol.NewComServer(COMPortName, COMBaudRate)
	for {
		err := com.ServeCOM()
		log.Printf("%v: Error: %v\n", COMPortName, err)
		log.Println("Retrying in 5s...")
		time.Sleep(time.Second * 5)
	}
}
