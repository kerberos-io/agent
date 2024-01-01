package models

type MotionDataPartial struct {
	Timestamp       int64 `json:"timestamp" bson:"timestamp"`
	NumberOfChanges int   `json:"numberOfChanges" bson:"numberOfChanges"`
}

type MotionDataFull struct {
	Timestamp       int64   `json:"timestamp" bson:"timestamp"`
	Size            float64 `json:"size" bson:"size"`
	Microseconds    float64 `json:"microseconds" bson:"microseconds"`
	DeviceName      string  `json:"deviceName" bson:"deviceName"`
	Region          string  `json:"region" bson:"region"`
	NumberOfChanges int     `json:"numberOfChanges" bson:"numberOfChanges"`
	Token           int     `json:"token" bson:"token"`
}
