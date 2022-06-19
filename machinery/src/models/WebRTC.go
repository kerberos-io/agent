package models

type SDPPayload struct {
	Cuuid    string `json:"cuuid"`
	Sdp      string `json:"sdp`
	CloudKey string `json:"cloud_key"`
}

type Candidate struct {
	Cuuid     string `json:"cuuid"`
	CloudKey  string `json:"cloud_key"`
	Candidate string `json:"candidate"`
}
