package models

type APIResponse struct {
	Data    interface{} `json:"data" bson:"data"`
	Message interface{} `json:"message" bson:"message"`
}

type OnvifCredentials struct {
	ONVIFXAddr    string `json:"onvif_xaddr,omitempty" bson:"onvif_xaddr"`
	ONVIFUsername string `json:"onvif_username,omitempty" bson:"onvif_username"`
	ONVIFPassword string `json:"onvif_password,omitempty" bson:"onvif_password"`
}

type CameraStreams struct {
	RTSP    string `json:"rtsp"`
	SubRTSP string `json:"sub_rtsp"`
}

type OnvifPanTilt struct {
	OnvifCredentials OnvifCredentials `json:"onvif_credentials,omitempty" bson:"onvif_credentials"`
	Pan              float64          `json:"pan,omitempty" bson:"pan"`
	Tilt             float64          `json:"tilt,omitempty" bson:"tilt"`
}

type OnvifZoom struct {
	OnvifCredentials OnvifCredentials `json:"onvif_credentials,omitempty" bson:"onvif_credentials"`
	Zoom             float64          `json:"zoom,omitempty" bson:"zoom"`
}
