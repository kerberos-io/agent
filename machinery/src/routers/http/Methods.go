package http

import (
	"github.com/gin-gonic/gin"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/onvif"
)

// Login godoc
// @Router /api/login [post]
// @ID login
// @Tags authentication
// @Summary Get Authorization token.
// @Description Get Authorization token.
// @Param credentials body models.Authentication true "Credentials"
// @Success 200 {object} models.Authorization
func Login() {}

// LoginToOnvif godoc
// @Router /api/camera/onvif/login [post]
// @ID camera-onvif-login
// @Tags camera
// @Param config body models.OnvifCredentials true "OnvifCredentials"
// @Summary Try to login into ONVIF supported camera.
// @Description Try to login into ONVIF supported camera.
// @Success 200 {object} models.APIResponse
func LoginToOnvif(c *gin.Context) {
	var onvifCredentials models.OnvifCredentials
	err := c.BindJSON(&onvifCredentials)

	if err == nil && onvifCredentials.ONVIFXAddr != "" {

		configuration := &models.Configuration{
			Config: models.Config{
				Capture: models.Capture{
					IPCamera: models.IPCamera{
						ONVIFXAddr:    onvifCredentials.ONVIFXAddr,
						ONVIFUsername: onvifCredentials.ONVIFUsername,
						ONVIFPassword: onvifCredentials.ONVIFPassword,
					},
				},
			},
		}

		device, err := onvif.ConnectToOnvifDevice(configuration)
		if err == nil {
			c.JSON(200, gin.H{
				"device": device,
			})
		} else {
			c.JSON(400, gin.H{
				"data": "Something went wrong: " + err.Error(),
			})
		}
	} else {
		c.JSON(400, gin.H{
			"data": "Something went wrong: " + err.Error(),
		})
	}
}

// GetOnvifCapabilities godoc
// @Router /api/camera/onvif/capabilities [post]
// @ID camera-onvif-capabilities
// @Tags camera
// @Param config body models.OnvifCredentials true "OnvifCredentials"
// @Summary Will return the ONVIF capabilities for the specific camera.
// @Description Will return the ONVIF capabilities for the specific camera.
// @Success 200 {object} models.APIResponse
func GetOnvifCapabilities(c *gin.Context) {
	var onvifCredentials models.OnvifCredentials
	err := c.BindJSON(&onvifCredentials)

	if err == nil && onvifCredentials.ONVIFXAddr != "" {

		configuration := &models.Configuration{
			Config: models.Config{
				Capture: models.Capture{
					IPCamera: models.IPCamera{
						ONVIFXAddr:    onvifCredentials.ONVIFXAddr,
						ONVIFUsername: onvifCredentials.ONVIFUsername,
						ONVIFPassword: onvifCredentials.ONVIFPassword,
					},
				},
			},
		}

		device, err := onvif.ConnectToOnvifDevice(configuration)
		if err == nil {
			c.JSON(200, gin.H{
				"capabilities": onvif.GetCapabilitiesFromDevice(device),
			})
		} else {
			c.JSON(400, gin.H{
				"data": "Something went wrong: " + err.Error(),
			})
		}
	} else {
		c.JSON(400, gin.H{
			"data": "Something went wrong: " + err.Error(),
		})
	}
}

// DoOnvifPanTilt godoc
// @Router /api/camera/onvif/pantilt [post]
// @ID camera-onvif-pantilt
// @Tags camera
// @Param panTilt body models.OnvifPanTilt true "OnvifPanTilt"
// @Summary Panning or/and tilting the camera.
// @Description Panning or/and tilting the camera using a direction (x,y).
// @Success 200 {object} models.APIResponse
func DoOnvifPanTilt(c *gin.Context) {
	var onvifPanTilt models.OnvifPanTilt
	err := c.BindJSON(&onvifPanTilt)

	if err == nil && onvifPanTilt.OnvifCredentials.ONVIFXAddr != "" {

		configuration := &models.Configuration{
			Config: models.Config{
				Capture: models.Capture{
					IPCamera: models.IPCamera{
						ONVIFXAddr:    onvifPanTilt.OnvifCredentials.ONVIFXAddr,
						ONVIFUsername: onvifPanTilt.OnvifCredentials.ONVIFUsername,
						ONVIFPassword: onvifPanTilt.OnvifCredentials.ONVIFPassword,
					},
				},
			},
		}

		device, err := onvif.ConnectToOnvifDevice(configuration)

		if err == nil {
			// Get token from the first profile
			token, err := onvif.GetTokenFromProfile(device, 0)

			if err == nil {

				// Get the configurations from the device
				configurations, err := onvif.GetConfigurationsFromDevice(device)

				if err == nil {

					pan := onvifPanTilt.Pan
					tilt := onvifPanTilt.Tilt
					err := onvif.ContinuousPanTilt(device, configurations, token, pan, tilt)
					if err == nil {
						c.JSON(200, models.APIResponse{
							Message: "Successfully pan/tilted the camera",
						})
					} else {
						c.JSON(400, models.APIResponse{
							Message: "Something went wrong: " + err.Error(),
						})
					}
				} else {
					c.JSON(400, models.APIResponse{
						Message: "Something went wrong: " + err.Error(),
					})
				}
			} else {
				c.JSON(400, models.APIResponse{
					Message: "Something went wrong: " + err.Error(),
				})
			}
		} else {
			c.JSON(400, models.APIResponse{
				Message: "Something went wrong: " + err.Error(),
			})
		}
	} else {
		c.JSON(400, models.APIResponse{
			Message: "Something went wrong: " + err.Error(),
		})
	}
}

// DoOnvifZoom godoc
// @Router /api/camera/onvif/zoom [post]
// @ID camera-onvif-zoom
// @Tags camera
// @Param zoom body models.OnvifZoom true "OnvifZoom"
// @Summary Zooming in or out the camera.
// @Description Zooming in or out the camera.
// @Success 200 {object} models.APIResponse
func DoOnvifZoom(c *gin.Context) {
	var onvifZoom models.OnvifZoom
	err := c.BindJSON(&onvifZoom)

	if err == nil && onvifZoom.OnvifCredentials.ONVIFXAddr != "" {

		configuration := &models.Configuration{
			Config: models.Config{
				Capture: models.Capture{
					IPCamera: models.IPCamera{
						ONVIFXAddr:    onvifZoom.OnvifCredentials.ONVIFXAddr,
						ONVIFUsername: onvifZoom.OnvifCredentials.ONVIFUsername,
						ONVIFPassword: onvifZoom.OnvifCredentials.ONVIFPassword,
					},
				},
			},
		}

		device, err := onvif.ConnectToOnvifDevice(configuration)

		if err == nil {
			// Get token from the first profile
			token, err := onvif.GetTokenFromProfile(device, 0)

			if err == nil {

				// Get the configurations from the device
				configurations, err := onvif.GetConfigurationsFromDevice(device)

				if err == nil {

					zoom := onvifZoom.Zoom
					err := onvif.ContinuousZoom(device, configurations, token, zoom)
					if err == nil {
						c.JSON(200, models.APIResponse{
							Message: "Successfully zoomed the camera",
						})
					} else {
						c.JSON(400, models.APIResponse{
							Message: "Something went wrong: " + err.Error(),
						})
					}
				} else {
					c.JSON(400, models.APIResponse{
						Message: "Something went wrong: " + err.Error(),
					})
				}
			} else {
				c.JSON(400, models.APIResponse{
					Message: "Something went wrong: " + err.Error(),
				})
			}
		} else {
			c.JSON(400, models.APIResponse{
				Message: "Something went wrong: " + err.Error(),
			})
		}
	} else {
		c.JSON(400, models.APIResponse{
			Message: "Something went wrong: " + err.Error(),
		})
	}
}
