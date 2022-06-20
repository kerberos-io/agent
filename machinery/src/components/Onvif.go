package components

import (
	"time"

	"github.com/cedricve/go-onvif"
	"github.com/kerberos-io/agent/machinery/src/log"
)

func Discover(timeout time.Duration) {
	log.Log.Info("Discovering devices")
	log.Log.Info("Waiting for " + (timeout * time.Second).String())
	devices, err := onvif.StartDiscovery(timeout * time.Second)
	if err != nil {
		log.Log.Error(err.Error())
	} else {
		for _, device := range devices {
			hostname, _ := device.GetHostname()
			log.Log.Info(hostname.Name)
		}
		if len(devices) == 0 {
			log.Log.Info("No devices descovered\n")
		}
	}
}
