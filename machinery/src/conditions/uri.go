package conditions

import (
	"github.com/kerberos-io/agent/machinery/src/models"
)

func IsValidConditionResponse(configuration *models.Configuration) bool {
	config := configuration.Config
	conditionURI := config.ConditionURI
	detectMotion := true
	if conditionURI != "" {

	}
	return detectMotion
}
