package http

import (
	"github.com/appleboy/gin-jwt/v2"
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/contrib/static"
	"github.com/gin-gonic/gin"
	_ "github.com/kerberos-io/opensource/backend/docs"
	"github.com/kerberos-io/opensource/backend/src/components"
	"github.com/kerberos-io/opensource/backend/src/models"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"log"
)

// @title Swagger Kerberos Open Source API
// @version 1.0
// @description This is the API for using and configure Kerberos Open source.
// @termsOfService https://kerberos.io

// @contact.name API Support
// @contact.url https://www.kerberos.io/support
// @contact.email support@kerberos.io

// @license.name Apache 2.0 - Commons Clause
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html

// @BasePath /

// @securityDefinitions.apikey Bearer
// @in header
// @name Authorization

var userConfig models.User

func StartServer(name string, port string){

	// Initialize REST API
	r := gin.Default()

	// Profile
	pprof.Register(r)

	// Setup CORS
	r.Use(CORS())

	// Serve frontend static files
	r.Use(static.Serve("/", static.LocalFile("./www", true)))

	// The JWT middleware
	middleWare := JWTMiddleWare()
	authMiddleware, err := jwt.New(&middleWare)
	if err != nil {
		log.Fatal("JWT Error:" + err.Error())
	}

	// Add Swagger
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Get the user configuration
	userConfig = components.ReadUserConfig()

	api := r.Group("/api")
	{
		// Bootstrap godoc
		// @Router /install [get]
		// @ID install
		// @Tags frontend
		// @Summary Get to know if the system was installed before or not.
		// @Description Get to know if the system was installed before or not.
		// @Success 200 {object} models.APIResponse

		api.GET("/install", func(c *gin.Context) {
			c.JSON(200, models.APIResponse {
				Data: userConfig.Installed,
			})
		})

		// Bootstrap godoc
		// @Router /install [post]
		// @ID install
		// @Tags frontend
		// @Summary If not yet installed, initiate the user configuration.
		// @Description If not yet installed, initiate the user configuration.
		// @Success 200 {object} models.APIResponse

		api.POST("/install", func(c *gin.Context) {
			// TODO update user config and update global object.
			// userConfig = ...
			c.JSON(200, models.APIResponse {
				Data: userConfig,
			})
		})

		api.Use(authMiddleware.MiddlewareFunc())
		{
			/*api.PUT("/configure", func(c *gin.Context) {
				c.JSON(200, gin.H{
					"configure": true,
				})
			})

			api.GET("/restart", func(c *gin.Context) {
				c.JSON(200, gin.H{
					"restart": true,
				})
			})

			api.GET("/motion", func(c *gin.Context) {
				c.JSON(200, gin.H{
					"data": "☄ Simulate motion",
				})
			})*/
		}
	}
	r.Run(":" + port)
}