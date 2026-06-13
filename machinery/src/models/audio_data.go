package models

type AudioDataPartial struct {
	Timestamp int64   `json:"timestamp" bson:"timestamp"`
	Data      []int16 `json:"data" bson:"data"`
}
