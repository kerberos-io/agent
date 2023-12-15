package http

import (
	jwt "github.com/appleboy/gin-jwt/v2"
	"github.com/gin-gonic/gin"
	"github.com/kerberos-io/agent/machinery/src/capture"
	"github.com/kerberos-io/agent/machinery/src/components"
	"github.com/kerberos-io/agent/machinery/src/onvif"
	"github.com/kerberos-io/agent/machinery/src/routers/websocket"

	"github.com/kerberos-io/agent/machinery/src/cloud"
	"github.com/kerberos-io/agent/machinery/src/models"
)

func AddRoutes(r *gin.Engine, authMiddleware *jwt.GinJWTMiddleware, configDirectory string, configuration *models.Configuration, communication *models.Communication, captureDevice *capture.Capture) *gin.RouterGroup {

	r.GET("/ws", func(c *gin.Context) {
		websocket.WebsocketHandler(c, communication, captureDevice)
	})

	// This is legacy should be removed in future! Now everything
	// lives under the /api prefix.
	r.GET("/config", func(c *gin.Context) {
		components.GetConfig(c, captureDevice, configuration, communication)
	})

	// This is legacy should be removed in future! Now everything
	// lives under the /api prefix.
	r.POST("/config", func(c *gin.Context) {
		components.UpdateConfig(c, configDirectory, configuration, communication)
	})

	api := r.Group("/api")
	{
		api.POST("/login", authMiddleware.LoginHandler)

		api.GET("/dashboard", func(c *gin.Context) {
			components.GetDashboard(c, configDirectory, configuration, communication)
		})

		api.POST("/latest-events", func(c *gin.Context) {
			components.GetLatestEvents(c, configDirectory, configuration, communication)
		})

		api.GET("/days", func(c *gin.Context) {
			components.GetDays(c, configDirectory, configuration, communication)
		})

		api.GET("/config", func(c *gin.Context) {
			components.GetConfig(c, captureDevice, configuration, communication)
		})

		api.POST("/config", func(c *gin.Context) {
			components.UpdateConfig(c, configDirectory, configuration, communication)
		})

		// Will verify the current onvif settings.
		api.POST("/onvif/verify", func(c *gin.Context) {
			onvif.VerifyOnvifConnection(c)
		})

		// Will verify the current hub settings.
		api.POST("/hub/verify", func(c *gin.Context) {
			cloud.VerifyHub(c)
		})

		// Will verify the current persistence settings.
		api.POST("/persistence/verify", func(c *gin.Context) {
			cloud.VerifyPersistence(c, configDirectory)
		})

		// Camera specific methods. Doesn't require any authorization.
		// These are available for anyone, but require the agent, to reach
		// the camera.

		api.POST("/camera/restart", func(c *gin.Context) {
			components.RestartAgent(c, communication)
		})

		api.POST("/camera/stop", func(c *gin.Context) {
			components.StopAgent(c, communication)
		})

		api.POST("/camera/record", func(c *gin.Context) {
			components.MakeRecording(c, communication)
		})

		api.POST("/camera/onvif/login", LoginToOnvif)
		api.POST("/camera/onvif/capabilities", GetOnvifCapabilities)
		api.POST("/camera/onvif/presets", GetOnvifPresets)
		api.POST("/camera/onvif/gotopreset", GoToOnvifPreset)
		api.POST("/camera/onvif/pantilt", DoOnvifPanTilt)
		api.POST("/camera/onvif/zoom", DoOnvifZoom)
		api.POST("/camera/onvif/inputs", DoGetDigitalInputs)
		api.POST("/camera/verify/:streamType", capture.VerifyCamera)

		// Secured endpoints..
		api.Use(authMiddleware.MiddlewareFunc())
		{
		}
	}
	return api
}
