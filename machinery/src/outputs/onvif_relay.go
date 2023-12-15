package outputs

import "github.com/kerberos-io/agent/machinery/src/models"

type OnvifRelayOutput struct {
	Output
}

func (o *OnvifRelayOutput) Trigger(message *models.OutputMessage) (err error) {
	err = nil
	return err
}
