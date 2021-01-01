package models


//Config is the highlevel struct which contains all the configuration of
//your Kerberos Open Source instance.
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

//Capture defines which camera type (Id) you are using (IP, USB or Raspberry Pi camera),
//and also contains recording specific parameters.
type Capture struct {
    Name                string        `json:"name"`
    IPCamera            IPCamera      `json:"ipcamera"`
    USBCamera           USBCamera     `json:"usbcamera"`
    RaspiCamera         RaspiCamera   `json:"raspicamera"`
    Continuous          string        `json:"continuous,omitempty"`
    PostRecording       int64         `json:"postrecording"`
    PreRecording        int           `json:"prerecording"`
    MaxLengthRecording  int64         `json:"maxlengthrecording"`
}

//IPCamera configuration, such as the RTSP url of the IPCamera and the FPS.
type IPCamera struct {
    RTSP          string        `json:"rtsp"`
    FPS           string        `json:"fps"`
}

//USBCamera configuration, such as the device path (/dev/video*)
type USBCamera struct {
    Device        string        `json:"device"`
}

//RaspiCamera configuration, such as the device path (/dev/video*)
type RaspiCamera struct {
    Device        string        `json:"device"`
}

//Region specifies the type (Id) of Region Of Interest (ROI), you
//would like to use.
type Region struct {
    Name          string        `json:"name"`
    Rectangle     Rectangle     `json:"rectangle"`
    Polygon       []Polygon     `json:"polygon"`
}

//Rectangle is defined by a starting point, left top (x1,y1) and end point (x2,y2).
type Rectangle struct {
    X1            int           `json:"x1"`
    Y1            int           `json:"y1"`
    X2            int           `json:"x2"`
    Y2            int           `json:"y2"`
}

//Polygon is a sequence of coordinates (x,y).
type Polygon struct {
    Id            string        `json:"id"`
    Coordinates   []Coordinate  `json:"coordinates"`
}

//Coordinate belongs to a Polygon.
type Coordinate struct {
    X             float64       `json:"x"`
    Y             float64       `json:"y"`
}

//Timetable allows you to set a Time Of Intterest (TOI), which limits recording or
//detection to a predefined time interval. Two tracks can be set, which allows you
//to give some flexibility.
type Timetable struct {
    Start1        int           `json:"start1"`
    End1          int           `json:"end1"`
    Start2        int           `json:"start2"`
    End2          int           `json:"end2"`
}

//KStorage contains the credentials of the Kerberos Storage/Kerberos Cloud instance.
//By defining KStorage you can make your recordings available in the cloud, at a centrel place.
type KStorage struct {
    URI               string        `json:"uri,omitempty" bson:"uri,omitempty"`
    CloudKey          string        `json:"cloud_key,omitempty" bson:"cloud_key,omitempty"`
    AccessKey         string        `json:"access_key,omitempty" bson:"access_key,omitempty"`
    SecretAccessKey   string        `json:"secret_access_key,omitempty" bson:"secret_access_key,omitempty"`
    Provider          string        `json:"provider,omitempty" bson:"provider,omitempty"`
    Directory         string        `json:"directory,omitempty" bson:"directory,omitempty"`
}
