package outputs

import (
	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
)

type Output interface {
	// Triggers the integration
	Trigger(message models.OutputMessage) error
}

func Execute(message *models.OutputMessage) (err error) {
	err = nil

	outputs := message.Outputs
	for _, output := range outputs {
		switch output {
		case "slack":
			slack := &SlackOutput{}
			err := slack.Trigger(message)
			if err == nil {
				log.Log.Debug("outputs.main.Execute(slack): message was processed by output.")
			} else {
				log.Log.Error("outputs.main.Execute(slack): " + err.Error())
			}
			break
		case "webhook":
			webhook := &WebhookOutput{}
			err := webhook.Trigger(message)
			if err == nil {
				log.Log.Debug("outputs.main.Execute(webhook): message was processed by output.")
			} else {
				log.Log.Error("outputs.main.Execute(webhook): " + err.Error())
			}
			break
		case "onvif_relay":
			onvif := &OnvifRelayOutput{}
			err := onvif.Trigger(message)
			if err == nil {
				log.Log.Debug("outputs.main.Execute(onvif): message was processed by output.")
			} else {
				log.Log.Error("outputs.main.Execute(onvif): " + err.Error())
			}
			break
		case "script":
			script := &ScriptOutput{}
			err := script.Trigger(message)
			if err == nil {
				log.Log.Debug("outputs.main.Execute(script): message was processed by output.")
			} else {
				log.Log.Error("outputs.main.Execute(script): " + err.Error())
			}
			break
		}
	}

	return err
}
