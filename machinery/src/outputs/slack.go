package outputs

import "github.com/kerberos-io/agent/machinery/src/models"

type SlackOutput struct {
	Output
}

func (s *SlackOutput) Trigger(message *models.OutputMessage) (err error) {
	err = nil
	return err
}
