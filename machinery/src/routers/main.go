package routers

import (
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/routers/http"
)

func StartWebserver(configuration *models.Configuration, communication *models.Communication) {
	http.StartServer(configuration, communication)
}
