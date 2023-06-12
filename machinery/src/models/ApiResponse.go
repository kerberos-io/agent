package models

type APIResponse struct {
	Data         interface{} `json:"data" bson:"data"`
	Message      interface{} `json:"message" bson:"message"`
	PTZFunctions interface{} `json:"ptz_functions" bson:"ptz_functions"`
	CanZoom      bool        `json:"can_zoom" bson:"can_zoom"`
	CanPanTilt   bool        `json:"can_pan_tilt" bson:"can_pan_tilt"`
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
