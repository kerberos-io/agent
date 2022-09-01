package models

type Media struct {
	Key        string `json:"key"`
	Path       string `json:"path"`
	Day        string `json:"day"`
	ShortDay   string `json:"short_day"`
	Time       string `json:"time"`
	Timestamp  string `json:"timestamp"`
	CameraName string `json:"camera_name"`
	CameraKey  string `json:"camera_key"`
}

type EventFilter struct {
	TimestampOffsetStart int64 `json:"timestamp_offset_start"`
	TimestampOffsetEnd   int64 `json:"timestamp_offset_end"`
	NumberOfElements     int   `json:"number_of_elements"`
}
