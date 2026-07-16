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

// DiscoveredDevice describes a device found on the local network during a
// discovery scan (fing/wifiman-style). It combines ONVIF WS-Discovery results
// with an active port scan and MAC/vendor lookup so cameras can be
// auto-detected and pre-filled in the configuration UI.
type DiscoveredDevice struct {
	IP           string       `json:"ip" bson:"ip"`
	Hostname     string       `json:"hostname,omitempty" bson:"hostname"`
	MAC          string       `json:"mac,omitempty" bson:"mac"`
	Vendor       string       `json:"vendor,omitempty" bson:"vendor"`
	Manufacturer string       `json:"manufacturer,omitempty" bson:"manufacturer"`
	Model        string       `json:"model,omitempty" bson:"model"`
	Type         string       `json:"type,omitempty" bson:"type"`
	Server       string       `json:"server,omitempty" bson:"server"`
	OpenPorts    []int        `json:"open_ports,omitempty" bson:"open_ports"`
	Services     []string     `json:"services,omitempty" bson:"services"`
	ONVIF        bool         `json:"onvif" bson:"onvif"`
	ONVIFXAddr   string       `json:"onvif_xaddr,omitempty" bson:"onvif_xaddr"`
	RTSPURL      string       `json:"rtsp_url,omitempty" bson:"rtsp_url"`
	RTSPStreams  []RTSPStream `json:"rtsp_streams,omitempty" bson:"rtsp_streams"`
	IsCamera     bool         `json:"is_camera" bson:"is_camera"`
	// IsAudio marks audio-only devices (e.g. IP speakers / intercoms such as
	// TOA) that expose RTSP to receive/stream audio rather than video.
	IsAudio bool `json:"is_audio" bson:"is_audio"`
}

// RTSPStream is a candidate RTSP stream URL for a discovered camera, derived
// from a built-in brand -> RTSP path mapping. When Verified is true the path was
// confirmed to exist on the device via an unauthenticated RTSP DESCRIBE probe
// (a 200 OK or a 401/403 "auth required" both prove the path is valid).
type RTSPStream struct {
	Brand        string `json:"brand,omitempty" bson:"brand"`
	Stream       string `json:"stream,omitempty" bson:"stream"` // "main" or "sub"
	Path         string `json:"path" bson:"path"`
	URL          string `json:"url" bson:"url"`
	Verified     bool   `json:"verified" bson:"verified"`
	RequiresAuth bool   `json:"requires_auth,omitempty" bson:"requires_auth"`
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

type OnvifPreset struct {
	OnvifCredentials OnvifCredentials `json:"onvif_credentials,omitempty" bson:"onvif_credentials"`
	Preset           string           `json:"preset,omitempty" bson:"preset"`
}
