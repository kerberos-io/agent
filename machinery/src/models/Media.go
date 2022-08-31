package models

type Media struct {
	Key        string `json:"key"`
	Path       string `json:"path"`
	Day        string `json:"day"`
	Time       string `json:"time"`
	Timestamp  string `json:"timestamp"`
	CameraName string `json:"camera_name"`
	CameraKey  string `json:"camera_key"`
}
