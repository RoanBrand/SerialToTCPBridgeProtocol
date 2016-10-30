package main

import (
	"encoding/json"
	"github.com/RoanBrand/SerialToTCPBridgeProtocol/protocol"
	"log"
	"os"
	"sync"
	"time"
)

func main() {
	c, err := loadConfig()
	if err != nil {
		log.Fatalf(`%v (You must have a valid "config.json" next to the executable)`, err)
	}
	if len(c.Gateways) == 0 {
		log.Fatal("No gateways configured in the config. Exiting.")
	}

	w := sync.WaitGroup{}
	for _, v := range c.Gateways {
		w.Add(1)
		go func(v gatewayConfig) {
			com := protocol.NewComGateway(v.COMPortName, v.COMBaudRate)
			for {
				err := com.ServeCOM()
				log.Printf("%v: Error: %v\nRetrying in 5s...\n", v.COMPortName, err)
				time.Sleep(time.Second * 5)
			}
			w.Done()
		}(v)
	}
	w.Wait()
}

type gatewayConfig struct {
	GatewayName string `json:"gateway name"`
	COMPortName string `json:"comport name"`
	COMBaudRate int    `json:"baud rate"`
}

type config struct {
	Gateways []gatewayConfig `json:"gateways"`
}

func loadConfig() (*config, error) {
	file, err := os.Open("config.json")
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(file)
	configuration := config{}
	err = decoder.Decode(&configuration)
	if err != nil {
		return nil, err
	}
	return &configuration, nil
}
