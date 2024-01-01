package conditions

import (
	"time"

	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
)

func IsWithinTimeInterval(loc *time.Location, configuration *models.Configuration) (enabled bool) {
	config := configuration.Config
	timeEnabled := config.Time
	enabled = true
	if timeEnabled != "false" {
		now := time.Now().In(loc)
		weekday := now.Weekday()
		hour := now.Hour()
		minute := now.Minute()
		second := now.Second()
		if config.Timetable != nil && len(config.Timetable) > 0 {
			timeInterval := config.Timetable[int(weekday)]
			if timeInterval != nil {
				start1 := timeInterval.Start1
				end1 := timeInterval.End1
				start2 := timeInterval.Start2
				end2 := timeInterval.End2
				currentTimeInSeconds := hour*60*60 + minute*60 + second
				if (currentTimeInSeconds >= start1 && currentTimeInSeconds <= end1) ||
					(currentTimeInSeconds >= start2 && currentTimeInSeconds <= end2) {
					log.Log.Debug("conditions.timewindow.IsWithinTimeInterval(): time interval valid, enabling recording.")
				} else {
					log.Log.Info("conditions.timewindow.IsWithinTimeInterval(): time interval not valid, disabling recording.")
					enabled = false
				}
			}
		}
	}
	return
}
