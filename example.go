package main

import (
	"encoding/json"
	"log"
	"os"
	"path"
	"sync"

	"github.com/RoanBrand/SerialToTCPBridgeProtocol/comwrapper"
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
			com := comwrapper.NewComPortGateway(v.COMPortName, v.COMBaudRate)
			com.ListenAndServe()
			w.Done()
		}(v)
	}
	w.Wait()
}

// Configuration by json file.
type gatewayConfig struct {
	GatewayName string `json:"gateway name"`
	COMPortName string `json:"comport name"`
	COMBaudRate int    `json:"baud rate"`
}

type config struct {
	Gateways []gatewayConfig `json:"gateways"`
}

func loadConfig() (*config, error) {
	filePath := "config.json"

	// if file not found in current WD, try executable's folder
	if !fileExists(filePath) {
		exePath, err := os.Executable()
		if err != nil {
			return nil, err
		}
		filePath = path.Join(path.Dir(exePath), filePath)
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}

	configuration := config{}
	err = json.NewDecoder(file).Decode(&configuration)
	if err != nil {
		return nil, err
	}
	return &configuration, nil
}

// fileExists checks if a file exists and is not a directory before we try using it to prevent further errors.
func fileExists(filePath string) bool {
	info, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}
