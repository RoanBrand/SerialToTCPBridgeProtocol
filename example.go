package main

import (
	"github.com/RoanBrand/SerialToTCPBridgeProtocol/protocol"
	"log"
	"time"
)

const (
	COMPortName = "COM4" // Windows
	//COMPortName = "/dev/serial0" // Linux
	COMBaudRate = 115200
)

func main() {
	com := protocol.NewComGateway(COMPortName, COMBaudRate)
	for {
		err := com.ServeCOM()
		log.Printf("%v: Error: %v\n", COMPortName, err)
		log.Println("Retrying in 5s...")
		time.Sleep(time.Second * 5)
	}
}
