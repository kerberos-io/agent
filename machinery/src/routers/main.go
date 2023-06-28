package routers

import (
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/routers/http"
)

func StartWebserver(configDirectory string, configuration *models.Configuration, communication *models.Communication) {
	http.StartServer(configDirectory, configuration, communication)
}
