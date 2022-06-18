package models

//Config is the highlevel struct which contains all the configuration of
//your Kerberos Open Source instance.
type Config struct {
	Type          string       `json:"type" binding:"required"`
	Key           string       `json:"key"`
	Name          string       `json:"name"`
	Time          string       `json:"time,omitempty" bson:"time"`
	Timezone      string       `json:"timezone,omitempty" bson:"timezone,omitempty"`
	Capture       Capture      `json:"capture"`
	Timetable     []*Timetable `json:"timetable"`
	Region        *Region      `json:"region"`
	Cloud         string       `json:"cloud,omitempty" bson:"cloud,omitempty"`
	S3            *S3          `json:"s3,omitempty" bson:"s3,omitempty"`
	KStorage      *KStorage    `json:"kstorage,omitempty" bson:"kstorage,omitempty"`
	MQTTURI       string       `json:"mqtturi,omitempty" bson:"mqtturi,omitempty"`
	MQTTUsername  string       `json:"mqtt_username,omitempty" bson:"mqtt_username"`
	MQTTPassword  string       `json:"mqtt_password,omitempty" bson:"mqtt_password"`
	STUNURI       string       `json:"stunuri,omitempty" bson:"stunuri"`
	TURNURI       string       `json:"turnuri,omitempty" bson:"turnuri"`
	TURNUsername  string       `json:"turn_username,omitempty" bson:"turn_username"`
	TURNPassword  string       `json:"turn_password,omitempty" bson:"turn_password"`
	HeartbeatURI  string       `json:"heartbeaturi,omitempty" bson:"heartbeaturi"` /*obsolete*/
	HubURI        string       `json:"hub_uri,omitempty" bson:"hub_uri"`
	HubKey        string       `json:"hub_key,omitempty" bson:"hub_key"`
	HubPrivateKey string       `json:"hub_private_key,omitempty" bson:"hub_private_key"`
	HubSite       string       `json:"hub_site,omitempty" bson:"hub_site"`
	ConditionURI  string       `json:"condition_uri,omitempty" bson:"condition_uri"`
}

//Capture defines which camera type (Id) you are using (IP, USB or Raspberry Pi camera),
//and also contains recording specific parameters.
type Capture struct {
	Name                  string      `json:"name"`
	IPCamera              IPCamera    `json:"ipcamera"`
	USBCamera             USBCamera   `json:"usbcamera"`
	RaspiCamera           RaspiCamera `json:"raspicamera"`
	Continuous            string      `json:"continuous,omitempty"`
	PostRecording         int64       `json:"postrecording"`
	PreRecording          int         `json:"prerecording"`
	MaxLengthRecording    int64       `json:"maxlengthrecording"`
	TranscodingWebRTC     string      `json:"transcodingwebrtc"`
	TranscodingResolution int64       `json:"transcodingresolution"`
	ForwardWebRTC         string      `json:"forwardwebrtc"`
	Fragmented            string      `json:"fragmented,omitempty" bson:"fragmented,omitempty"`
	FragmentedDuration    int64       `json:"fragmentedduration,omitempty" bson:"fragmentedduration,omitempty"`
}

//IPCamera configuration, such as the RTSP url of the IPCamera and the FPS.
//Also includes ONVIF integration
type IPCamera struct {
	RTSP          string `json:"rtsp"`
	FPS           string `json:"fps"`
	ONVIF         bool   `json:"onvif,omitempty" bson:"onvif"`
	ONVIFXAddr    string `json:"onvif_xaddr,omitempty" bson:"onvif_xaddr"`
	ONVIFUsername string `json:"onvif_username,omitempty" bson:"onvif_username"`
	ONVIFPassword string `json:"onvif_password,omitempty" bson:"onvif_password"`
}

//USBCamera configuration, such as the device path (/dev/video*)
type USBCamera struct {
	Device string `json:"device"`
}

//RaspiCamera configuration, such as the device path (/dev/video*)
type RaspiCamera struct {
	Device string `json:"device"`
}

//Region specifies the type (Id) of Region Of Interest (ROI), you
//would like to use.
type Region struct {
	Name      string    `json:"name"`
	Rectangle Rectangle `json:"rectangle"`
	Polygon   []Polygon `json:"polygon"`
}

//Rectangle is defined by a starting point, left top (x1,y1) and end point (x2,y2).
type Rectangle struct {
	X1 int `json:"x1"`
	Y1 int `json:"y1"`
	X2 int `json:"x2"`
	Y2 int `json:"y2"`
}

//Polygon is a sequence of coordinates (x,y). The ID specifies an unique identifier,
//as multiple polygons can be defined.
type Polygon struct {
	ID          string       `json:"id"`
	Coordinates []Coordinate `json:"coordinates"`
}

//Coordinate belongs to a Polygon.
type Coordinate struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

//Timetable allows you to set a Time Of Intterest (TOI), which limits recording or
//detection to a predefined time interval. Two tracks can be set, which allows you
//to give some flexibility.
type Timetable struct {
	Start1 int `json:"start1"`
	End1   int `json:"end1"`
	Start2 int `json:"start2"`
	End2   int `json:"end2"`
}

//S3 integration
type S3 struct {
	Proxy     string `json:"proxy,omitempty" bson:"proxy,omitempty"`
	ProxyURI  string `json:"proxyuri,omitempty" bson:"proxyuri,omitempty"`
	Bucket    string `json:"bucket,omitempty" bson:"bucket,omitempty"`
	Region    string `json:"region,omitempty" bson:"region,omitempty"`
	Username  string `json:"username,omitempty" bson:"username,omitempty"`
	Publickey string `json:"publickey,omitempty" bson:"publickey,omitempty"`
	Secretkey string `json:"secretkey,omitempty" bson:"secretkey,omitempty"`
}

//KStorage contains the credentials of the Kerberos Storage/Kerberos Cloud instance.
//By defining KStorage you can make your recordings available in the cloud, at a centrel place.
type KStorage struct {
	URI             string `json:"uri,omitempty" bson:"uri,omitempty"`
	CloudKey        string `json:"cloud_key,omitempty" bson:"cloud_key,omitempty"`
	AccessKey       string `json:"access_key,omitempty" bson:"access_key,omitempty"`
	SecretAccessKey string `json:"secret_access_key,omitempty" bson:"secret_access_key,omitempty"`
	Provider        string `json:"provider,omitempty" bson:"provider,omitempty"`
	Directory       string `json:"directory,omitempty" bson:"directory,omitempty"`
}
