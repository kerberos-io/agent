package conditions

import (
	"errors"
	"time"

	"github.com/kerberos-io/agent/machinery/src/models"
)

func Validate(loc *time.Location, configuration *models.Configuration) (valid bool, err error) {
	valid = true
	err = nil

	withinTimeInterval := IsWithinTimeInterval(loc, configuration)
	if !withinTimeInterval {
		valid = false
		err = errors.New("time interval not valid")
		return
	}
	validUriResponse := IsValidUriResponse(configuration)
	if !validUriResponse {
		valid = false
		err = errors.New("uri response not valid")
		return
	}

	return
}
