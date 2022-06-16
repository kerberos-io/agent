package http

import (
	jwt "github.com/appleboy/gin-jwt/v2"
	"github.com/gin-gonic/gin"
	"github.com/kerberos-io/agent/machinery/src/components"
	"github.com/kerberos-io/agent/machinery/src/models"
)

func AddRoutes(r *gin.Engine, authMiddleware *jwt.GinJWTMiddleware, config *models.Config, customConfig *models.Config, globalConfig *models.Config) *gin.RouterGroup {
	api := r.Group("/api")
	{
		api.POST("/login", authMiddleware.LoginHandler)
		api.GET("/install", GetInstallation)
		api.POST("/install", UpdateInstallation)

		api.GET("/config", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"config":   config,
				"custom":   customConfig,
				"global":   globalConfig,
				"snapshot": components.GetSnapshot(),
			})
		})

		api.Use(authMiddleware.MiddlewareFunc())
		{
			// Secured endpoints..

		}
	}
	return api
}

// GetInstallation example
// @Summary Get to know if the system was installed before or not.
// @Description Get to know if the system was installed before or not.
// @ID web.getinstallation
// @Produce json
// @Success 200 {object} models.APIResponse
// @Router /api/install [get]
func GetInstallation(c *gin.Context) {
	// Get the user configuration
	userConfig := components.ReadUserConfig()

	c.JSON(200, models.APIResponse{
		Data: userConfig.Installed,
	})
}

// UpdateInstallation example
// @Summary If not yet installed, initiate the user configuration.
// @Description If not yet installed, initiate the user configuration.
// @ID web.updateinstallation
// @Produce json
// @Success 200 {object} models.APIResponse
// @Router /api/install [post]
func UpdateInstallation(c *gin.Context) {
	// TODO update user config and update global object.
	// userConfig = ...
	userConfig := components.ReadUserConfig()
	c.JSON(200, models.APIResponse{
		Data: userConfig,
	})
}
