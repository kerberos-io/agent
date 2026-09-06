package http

import (
	"math"
	stdhttp "net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/utils"
)

const (
	healthApplicationStatus = "get_success"
	healthEntityStatus      = "healthy"
	hubConnectionFreshness  = 3 * time.Minute
)

// StreamResolution is the most recently observed encoded video size in pixels.
type StreamResolution struct {
	Width  int64 `json:"width"`
	Height int64 `json:"height"`
}

// StreamHealth describes the most recently observed state of one camera stream.
type StreamHealth struct {
	Configured        bool             `json:"configured"`
	Connected         bool             `json:"connected"`
	PackagesProcessed uint64           `json:"packagesProcessed"`
	FPS               float64          `json:"fps"`
	Resolution        StreamResolution `json:"resolution"`
	LastPacketAt      int64            `json:"lastPacketAt"`
}

// HubHealth describes the Agent's periodic HTTP heartbeat connection to Hub.
type HubHealth struct {
	Configured                bool  `json:"configured"`
	Connected                 bool  `json:"connected"`
	LastHeartbeatAttemptAt    int64 `json:"lastHeartbeatAttemptAt"`
	LastSuccessfulHeartbeatAt int64 `json:"lastSuccessfulHeartbeatAt"`
}

// Health describes the Agent process health exposed to API clients.
type Health struct {
	Description     string       `json:"description"`
	CameraConnected bool         `json:"cameraConnected"`
	MainStream      StreamHealth `json:"mainStream"`
	SubStream       StreamHealth `json:"subStream"`
	Hub             HubHealth    `json:"hub"`
}

// HealthResponseData contains the typed payload of a health response.
type HealthResponseData struct {
	Health Health `json:"health"`
}

// HealthResponse is the standard successful response returned by GET /health.
type HealthResponse struct {
	SuccessResponse
	Data HealthResponseData `json:"data"`
}

// HealthCheck godoc
// @Summary Check Agent health
// @Description Confirms that the Agent HTTP process can serve requests and reports current camera stream and Hub heartbeat diagnostics. Operational dependency failures do not change the liveness HTTP status.
// @Tags health
// @Produce json
// @Success 200 {object} HealthResponse
// @Router /health [get]
func HealthCheck(c *gin.Context, communication *models.Communication) {
	c.Header("Cache-Control", "no-store")
	c.JSON(stdhttp.StatusOK, HealthResponse{
		SuccessResponse: NewSuccessResponse(
			stdhttp.StatusOK,
			healthApplicationStatus,
			healthEntityStatus,
			"Healthy",
			ResponseMetadata{
				ApplicationName:    "agent",
				ApplicationVersion: utils.VERSION,
				Path:               c.Request.URL.Path,
			},
		),
		Data: HealthResponseData{
			Health: buildHealth(communication, time.Now()),
		},
	})
}

func buildHealth(communication *models.Communication, now time.Time) Health {
	mainTelemetry := communication.StreamRuntimeTelemetry(models.MainStream)
	subTelemetry := communication.StreamRuntimeTelemetry(models.SubStream)
	hubTelemetry := communication.HubRuntimeTelemetry()

	hubConnected := hubTelemetry.Configured &&
		hubTelemetry.Connected &&
		hubTelemetry.LastSuccessfulHeartbeatAt > 0 &&
		now.Unix()-hubTelemetry.LastSuccessfulHeartbeatAt <= int64(hubConnectionFreshness/time.Second)

	return Health{
		Description:     "Agent HTTP service is healthy",
		CameraConnected: communication.CameraConnected.Load(),
		MainStream:      newStreamHealth(mainTelemetry, communication.MainStreamConnected.Load()),
		SubStream:       newStreamHealth(subTelemetry, communication.SubStreamConnected.Load()),
		Hub: HubHealth{
			Configured:                hubTelemetry.Configured,
			Connected:                 hubConnected,
			LastHeartbeatAttemptAt:    hubTelemetry.LastHeartbeatAttemptAt,
			LastSuccessfulHeartbeatAt: hubTelemetry.LastSuccessfulHeartbeatAt,
		},
	}
}

func newStreamHealth(telemetry models.StreamRuntimeTelemetry, connected bool) StreamHealth {
	return StreamHealth{
		Configured:        telemetry.Configured,
		Connected:         connected,
		PackagesProcessed: telemetry.PackagesProcessed,
		FPS:               math.Round(telemetry.FPS*100) / 100,
		Resolution: StreamResolution{
			Width:  telemetry.Width,
			Height: telemetry.Height,
		},
		LastPacketAt: telemetry.LastPacketAt,
	}
}
