package routers

import (
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/routers/http"
	"github.com/kerberos-io/agent/machinery/src/routers/mqtt"
)

func StartWebserver(name string, port string, config *models.Config, customConfig *models.Config, globalConfig *models.Config) {
	http.StartServer(name, port, config, customConfig, globalConfig)
}

func StartMqttListener(name string) {
	mqtt.StartListener(name)
}
