package outputs

import "github.com/kerberos-io/agent/machinery/src/models"

type ScriptOutput struct {
	Output
}

func (scr *ScriptOutput) Trigger(message *models.OutputMessage) (err error) {
	err = nil
	return err
}
