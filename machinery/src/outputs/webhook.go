package outputs

import "github.com/kerberos-io/agent/machinery/src/models"

type WebhookOutput struct {
	Output
}

func (w *WebhookOutput) Trigger(message *models.OutputMessage) (err error) {
	err = nil
	return err
}
