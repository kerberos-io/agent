package http

import (
	"github.com/gin-gonic/gin"
	"github.com/kerberos-io/agent/machinery/src/log"
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

		cameraConfiguration := configuration.Config.Capture.IPCamera
		device, err := onvif.ConnectToOnvifDevice(&cameraConfiguration)
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

		cameraConfiguration := configuration.Config.Capture.IPCamera
		device, err := onvif.ConnectToOnvifDevice(&cameraConfiguration)
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

		cameraConfiguration := configuration.Config.Capture.IPCamera
		device, err := onvif.ConnectToOnvifDevice(&cameraConfiguration)

		if err == nil {
			// Get token from the first profile
			token, err := onvif.GetTokenFromProfile(device, 0)

			if err == nil {

				// Get the configurations from the device
				ptzConfigurations, err := onvif.GetPTZConfigurationsFromDevice(device)

				if err == nil {

					pan := onvifPanTilt.Pan
					tilt := onvifPanTilt.Tilt
					err := onvif.ContinuousPanTilt(device, ptzConfigurations, token, pan, tilt)
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

		cameraConfiguration := configuration.Config.Capture.IPCamera
		device, err := onvif.ConnectToOnvifDevice(&cameraConfiguration)

		if err == nil {
			// Get token from the first profile
			token, err := onvif.GetTokenFromProfile(device, 0)

			if err == nil {

				// Get the PTZ configurations from the device
				ptzConfigurations, err := onvif.GetPTZConfigurationsFromDevice(device)

				if err == nil {

					zoom := onvifZoom.Zoom
					err := onvif.ContinuousZoom(device, ptzConfigurations, token, zoom)
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

// GetOnvifPresets godoc
// @Router /api/camera/onvif/presets [post]
// @ID camera-onvif-presets
// @Tags camera
// @Param config body models.OnvifCredentials true "OnvifCredentials"
// @Summary Will return the ONVIF presets for the specific camera.
// @Description Will return the ONVIF presets for the specific camera.
// @Success 200 {object} models.APIResponse
func GetOnvifPresets(c *gin.Context) {
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

		cameraConfiguration := configuration.Config.Capture.IPCamera
		device, err := onvif.ConnectToOnvifDevice(&cameraConfiguration)
		if err == nil {
			presets, err := onvif.GetPresetsFromDevice(device)
			if err == nil {
				c.JSON(200, gin.H{
					"presets": presets,
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
	} else {
		c.JSON(400, gin.H{
			"data": "Something went wrong: " + err.Error(),
		})
	}
}

// GoToOnvifPReset godoc
// @Router /api/camera/onvif/gotopreset [post]
// @ID camera-onvif-gotopreset
// @Tags camera
// @Param config body models.OnvifPreset true "OnvifPreset"
// @Summary Will activate the desired ONVIF preset.
// @Description Will activate the desired ONVIF preset.
// @Success 200 {object} models.APIResponse
func GoToOnvifPreset(c *gin.Context) {
	var onvifPreset models.OnvifPreset
	err := c.BindJSON(&onvifPreset)

	if err == nil && onvifPreset.OnvifCredentials.ONVIFXAddr != "" {

		configuration := &models.Configuration{
			Config: models.Config{
				Capture: models.Capture{
					IPCamera: models.IPCamera{
						ONVIFXAddr:    onvifPreset.OnvifCredentials.ONVIFXAddr,
						ONVIFUsername: onvifPreset.OnvifCredentials.ONVIFUsername,
						ONVIFPassword: onvifPreset.OnvifCredentials.ONVIFPassword,
					},
				},
			},
		}

		cameraConfiguration := configuration.Config.Capture.IPCamera
		device, err := onvif.ConnectToOnvifDevice(&cameraConfiguration)
		if err == nil {
			err := onvif.GoToPresetFromDevice(device, onvifPreset.Preset)
			if err == nil {
				c.JSON(200, gin.H{
					"data": "Camera preset activated: " + onvifPreset.Preset,
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
	} else {
		c.JSON(400, gin.H{
			"data": "Something went wrong: " + err.Error(),
		})
	}
}

// DoGetDigitalInputs godoc
// @Router /api/camera/onvif/inputs [post]
// @ID get-digital-inputs
// @Security Bearer
// @securityDefinitions.apikey Bearer
// @in header
// @name Authorization
// @Tags camera
// @Param config body models.OnvifCredentials true "OnvifCredentials"
// @Summary Will get the digital inputs from the ONVIF device.
// @Description Will get the digital inputs from the ONVIF device.
// @Success 200 {object} models.APIResponse
func DoGetDigitalInputs(c *gin.Context) {
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

		cameraConfiguration := configuration.Config.Capture.IPCamera
		_, err := onvif.ConnectToOnvifDevice(&cameraConfiguration)
		if err == nil {
			// Get the digital inputs and outputs from the device
			inputOutputs, err := onvif.GetInputOutputs()
			if err == nil {
				if err == nil {
					// Get the digital outputs from the device
					var inputs []onvif.ONVIFEvents
					for _, event := range inputOutputs {
						if event.Type == "input" {
							inputs = append(inputs, event)
						}
					}
					c.JSON(200, gin.H{
						"data": inputs,
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

// DoGetRelayOutputs godoc
// @Router /api/camera/onvif/outputs [post]
// @ID get-relay-outputs
// @Security Bearer
// @securityDefinitions.apikey Bearer
// @in header
// @name Authorization
// @Tags camera
// @Param config body models.OnvifCredentials true "OnvifCredentials"
// @Summary Will get the relay outputs from the ONVIF device.
// @Description Will get the relay outputs from the ONVIF device.
// @Success 200 {object} models.APIResponse
func DoGetRelayOutputs(c *gin.Context) {
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

		cameraConfiguration := configuration.Config.Capture.IPCamera
		_, err := onvif.ConnectToOnvifDevice(&cameraConfiguration)
		if err == nil {
			// Get the digital inputs and outputs from the device
			inputOutputs, err := onvif.GetInputOutputs()
			if err == nil {
				if err == nil {
					// Get the digital outputs from the device
					var outputs []onvif.ONVIFEvents
					for _, event := range inputOutputs {
						if event.Type == "output" {
							outputs = append(outputs, event)
						}
					}
					c.JSON(200, gin.H{
						"data": outputs,
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

// DoTriggerRelayOutput godoc
// @Router /api/camera/onvif/outputs/{output} [post]
// @ID trigger-relay-output
// @Security Bearer
// @securityDefinitions.apikey Bearer
// @in header
// @name Authorization
// @Tags camera
// @Param config body models.OnvifCredentials true "OnvifCredentials"
// @Param output path string true "Output"
// @Summary Will trigger the relay output from the ONVIF device.
// @Description Will trigger the relay output from the ONVIF device.
// @Success 200 {object} models.APIResponse
func DoTriggerRelayOutput(c *gin.Context) {
	var onvifCredentials models.OnvifCredentials
	err := c.BindJSON(&onvifCredentials)

	// Get the output from the url
	output := c.Param("output")

	if err == nil && onvifCredentials.ONVIFXAddr != "" && output != "" {

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

		cameraConfiguration := configuration.Config.Capture.IPCamera
		device, err := onvif.ConnectToOnvifDevice(&cameraConfiguration)
		if err == nil {
			err := onvif.TriggerRelayOutput(device, output)
			if err == nil {
				msg := "relay output triggered: " + output
				log.Log.Info("routers.http.methods.DoTriggerRelayOutput(): " + msg)
				c.JSON(200, gin.H{
					"data": msg,
				})
			} else {
				msg := "something went wrong: " + err.Error()
				log.Log.Error("routers.http.methods.DoTriggerRelayOutput(): " + msg)
				c.JSON(400, gin.H{
					"data": msg,
				})
			}
		} else {
			msg := "something went wrong: " + err.Error()
			log.Log.Error("routers.http.methods.DoTriggerRelayOutput(): " + msg)
			c.JSON(400, gin.H{
				"data": msg,
			})
		}
	} else {
		msg := "something went wrong: " + err.Error()
		log.Log.Error("routers.http.methods.DoTriggerRelayOutput(): " + msg)
		c.JSON(400, gin.H{
			"data": msg,
		})
	}
}
