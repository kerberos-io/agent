package http

import (
	"encoding/json"
	"io/ioutil"
	"os"

	jwt "github.com/appleboy/gin-jwt/v2"
	"github.com/gin-gonic/gin"
	"gopkg.in/mgo.v2/bson"

	"github.com/kerberos-io/agent/machinery/src/components"
	"github.com/kerberos-io/agent/machinery/src/database"
	"github.com/kerberos-io/agent/machinery/src/models"
)

func AddRoutes(r *gin.Engine, authMiddleware *jwt.GinJWTMiddleware, configuration *models.Configuration, communication *models.Communication) *gin.RouterGroup {

	r.GET("/config", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"config":   configuration.Config,
			"custom":   configuration.CustomConfig,
			"global":   configuration.GlobalConfig,
			"snapshot": components.GetSnapshot(),
		})
	})

	r.POST("/config", func(c *gin.Context) {
		if !communication.IsConfiguring.IsSet() {
			communication.IsConfiguring.Set()

			// Save into file
			var conf models.Config
			var confMap map[string]interface{}
			c.BindJSON(&confMap)
			inrec, _ := json.Marshal(confMap)
			json.Unmarshal(inrec, &conf)

			if os.Getenv("DEPLOYMENT") == "" || os.Getenv("DEPLOYMENT") == "agent" {
				res, _ := json.MarshalIndent(conf, "", "\t")
				ioutil.WriteFile("./data/config/config.json", res, 0644)
			} else if os.Getenv("DEPLOYMENT") == "factory" {
				// Write to mongodb
				session := database.New().Copy()
				defer session.Close()
				db := session.DB(database.DatabaseName)
				collection := db.C("configuration")

				collection.Update(bson.M{
					"type": "config",
					"name": os.Getenv("DEPLOYMENT_NAME"),
				}, &conf)
			}

			configuration.Config = conf       // HACK
			configuration.CustomConfig = conf // HACK

			communication.HandleBootstrap <- "restart"
			communication.IsConfiguring.UnSet()

			c.JSON(200, gin.H{
				"data":   "☄ Reconfiguring",
				"config": conf,
				"custom": conf,
			})
		} else {
			c.JSON(200, gin.H{
				"data": "☄ Already reconfiguring",
			})
		}
	})

	api := r.Group("/api")
	{
		api.POST("/login", authMiddleware.LoginHandler)

		api.GET("/config", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"config":   configuration.Config,
				"custom":   configuration.CustomConfig,
				"global":   configuration.GlobalConfig,
				"snapshot": components.GetSnapshot(),
			})
		})

		api.GET("/restart", func(c *gin.Context) {
			communication.HandleBootstrap <- "restart"
			c.JSON(200, gin.H{
				"restarted": true,
			})
		})

		api.GET("/stop", func(c *gin.Context) {
			communication.HandleBootstrap <- "stop"
			c.JSON(200, gin.H{
				"stopped": true,
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
