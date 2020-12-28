package models

type Config struct {
    Type  		      string        `json:"type" binding:"required"`
    Key               string        `json:"key"`
    Name              string        `json:"name"`
    Timezone          string        `json:"timezone,omitempty" bson:"timezone,omitempty"`
    Capture           Capture       `json:"capture"`
    Timetable         []*Timetable  `json:"timetable"`
    Region            *Region       `json:"region"`
    Cloud             string        `json:"cloud,omitempty" bson:"cloud,omitempty"`
    KStorage          *KStorage     `json:"kstorage,omitempty" bson:"kstorage,omitempty"`
    MQTTURI           string        `json:"mqtturi,omitempty" bson:"mqtturi,omitempty"`
}

type Capture struct {
    Id                  string        `json:"id"`
    IPCamera            IPCamera      `json:"ipcamera"`
    Continuous          string        `json:"continuous,omitempty"`
    PostRecording       int64         `json:"postrecording"`
    PreRecording        int           `json:"prerecording"`
    MaxLengthRecording  int64         `json:"maxlengthrecording"`
}

type IPCamera struct {
    RTSP          string        `json:"rtsp"`
    FPS           string        `json:"fps"`
}

type Region struct {
    Rectangle     Rectangle     `json:"rectangle"`
    Polygon       []Polygon     `json:"polygon"`
}

type Rectangle struct {
    X1            int           `json:"x1"`
    Y1            int           `json:"y1"`
    X2            int           `json:"x2"`
    Y2            int           `json:"y2"`
}

type Polygon struct {
    Id            string        `json:"id"`
    Coordinates   []Coordinate  `json:"coordinates"`
}

type Coordinate struct {
    X             float64       `json:"x"`
    Y             float64       `json:"y"`
}

type Timetable struct {
    Start1        int           `json:"start1"`
    End1          int           `json:"end1"`
    Start2        int           `json:"start2"`
    End2          int           `json:"end2"`
}

type KStorage struct {
    URI               string        `json:"uri,omitempty" bson:"uri,omitempty"`
    CloudKey          string        `json:"cloud_key,omitempty" bson:"cloud_key,omitempty"`
    AccessKey         string        `json:"access_key,omitempty" bson:"access_key,omitempty"`
    SecretAccessKey   string        `json:"secret_access_key,omitempty" bson:"secret_access_key,omitempty"`
    Provider          string        `json:"provider,omitempty" bson:"provider,omitempty"`
    Directory         string        `json:"directory,omitempty" bson:"directory,omitempty"`
}
