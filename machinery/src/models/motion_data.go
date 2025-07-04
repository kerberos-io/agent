package models

type MotionDataPartial struct {
	Timestamp       int64           `json:"timestamp" bson:"timestamp"`
	NumberOfChanges int             `json:"numberOfChanges" bson:"numberOfChanges"`
	Rectangle       MotionRectangle `json:"rectangle" bson:"rectangle"`
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

type MotionRectangle struct {
	X      int `json:"x" bson:"x"`
	Y      int `json:"y" bson:"y"`
	Width  int `json:"width" bson:"width"`
	Height int `json:"height" bson:"height"`
}
