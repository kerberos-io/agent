package models

import "time"

// The OutputMessage contains the relevant information
// to specify the type of triggers we want to execute.
type OutputMessage struct {
	Name      string
	Outputs   []string
	Trigger   string
	Timestamp time.Time
	File      string
	CameraId  string
	SiteId    string
}
