package routers

import (
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/routers/http"
	"github.com/kerberos-io/agent/machinery/src/routers/mqtt"
)

func StartWebserver(configuration *models.Configuration, communication *models.Communication) {
	http.StartServer(configuration, communication)
}

func StartMqttListener(name string) {
	mqtt.StartListener(name)
}
