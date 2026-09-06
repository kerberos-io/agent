package http

import "time"

// ResponseMetadata contains request and application context shared by API responses.
type ResponseMetadata struct {
	ApplicationName    string `json:"applicationName,omitempty"`
	ApplicationVersion string `json:"applicationVersion,omitempty"`
	Timestamp          int64  `json:"timestamp,omitempty"`
	Path               string `json:"path,omitempty"`
	TraceID            string `json:"traceId,omitempty"`
}

// SuccessResponse is the common envelope for successful Agent API responses.
type SuccessResponse struct {
	HTTPStatusCode        int              `json:"httpStatusCode"`
	ApplicationStatusCode string           `json:"applicationStatusCode"`
	EntityStatusCode      string           `json:"entityStatusCode"`
	Message               string           `json:"message"`
	Metadata              ResponseMetadata `json:"metadata"`
}

// NewSuccessResponse creates the Hub-compatible success envelope used by new
// Agent API endpoints.
func NewSuccessResponse(
	httpStatusCode int,
	applicationStatusCode string,
	entityStatusCode string,
	message string,
	metadata ResponseMetadata,
) SuccessResponse {
	metadata.Timestamp = time.Now().Unix()

	return SuccessResponse{
		HTTPStatusCode:        httpStatusCode,
		ApplicationStatusCode: applicationStatusCode,
		EntityStatusCode:      entityStatusCode,
		Message:               message,
		Metadata:              metadata,
	}
}
