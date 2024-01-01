package routers

import (
	"github.com/kerberos-io/agent/machinery/src/capture"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/routers/http"
)

func StartWebserver(configDirectory string, configuration *models.Configuration, communication *models.Communication, captureDevice *capture.Capture) {
	http.StartServer(configDirectory, configuration, communication, captureDevice)
}
