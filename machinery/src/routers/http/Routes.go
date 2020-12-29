package http

import (
	"github.com/appleboy/gin-jwt/v2"
	"github.com/gin-gonic/gin"
	"github.com/kerberos-io/opensource/machinery/src/components"
	"github.com/kerberos-io/opensource/machinery/src/models"
)

func AddRoutes(r *gin.Engine, authMiddleware *jwt.GinJWTMiddleware ) {

	api := r.Group("/api")
	{
		api.GET("/install", GetInstallation)
		api.POST("/install", UpdateInstallation)

		api.Use(authMiddleware.MiddlewareFunc())
		{

		}
	}
}

// GetInstallation godoc
// @Router /api/install [get]
// @ID installation
// @Tags web
// @Summary Get to know if the system was installed before or not.
// @Description Get to know if the system was installed before or not.
// @Success 200 {object} models.APIResponse

func GetInstallation(c *gin.Context) {
	// Get the user configuration
	userConfig := components.ReadUserConfig()

	c.JSON(200, models.APIResponse {
		Data: userConfig.Installed,
	})
}

// UpdateInstallation godoc
// @Router /api/install [post]
// @ID update-installation
// @Tags web
// @Summary If not yet installed, initiate the user configuration.
// @Description If not yet installed, initiate the user configuration.
// @Success 200 {object} models.APIResponse

func UpdateInstallation (c *gin.Context) {
	// TODO update user config and update global object.
	// userConfig = ...
	userConfig := components.ReadUserConfig()

	c.JSON(200, models.APIResponse {
		Data: userConfig,
	})
}