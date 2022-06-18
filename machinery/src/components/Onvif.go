package components

import (
	"time"

	"github.com/cedricve/go-onvif"
)

func Discover(log Logging, timeout time.Duration) {
	log.Info("Discovering devices")
	log.Info("Waiting for " + (timeout * time.Second).String())
	devices, err := onvif.StartDiscovery(timeout * time.Second)
	if err != nil {
		log.Error(err.Error())
	} else {
		for _, device := range devices {
			hostname, _ := device.GetHostname()
			log.Info(hostname.Name)
		}
		if len(devices) == 0 {
			log.Info("No devices descovered\n")
		}
	}
}
