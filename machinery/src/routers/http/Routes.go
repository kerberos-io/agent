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
			err := c.BindJSON(&conf)
			if err == nil {
				if os.Getenv("DEPLOYMENT") == "factory" || os.Getenv("MACHINERY_ENVIRONMENT") == "kubernetes" {
					// Write to mongodb
					session := database.New().Copy()
					defer session.Close()
					db := session.DB(database.DatabaseName)
					collection := db.C("configuration")

					collection.Update(bson.M{
						"type": "config",
						"name": os.Getenv("DEPLOYMENT_NAME"),
					}, &conf)
				} else if os.Getenv("DEPLOYMENT") == "" || os.Getenv("DEPLOYMENT") == "agent" {
					res, _ := json.MarshalIndent(conf, "", "\t")
					ioutil.WriteFile("./data/config/config.json", res, 0644)
				}

				select {
				case communication.HandleBootstrap <- "restart":
				default:
				}

				communication.IsConfiguring.UnSet()

				c.JSON(200, gin.H{
					"data": "☄ Reconfiguring",
				})
			} else {
				c.JSON(400, gin.H{
					"data": "Something went wrong: " + err.Error(),
				})
			}
		} else {
			c.JSON(400, gin.H{
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
