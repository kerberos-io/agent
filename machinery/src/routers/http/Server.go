package http

import (
	jwt "github.com/appleboy/gin-jwt/v2"
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/contrib/static"
	"github.com/gin-gonic/gin"

	//Swagger documentantion
	"log"

	_ "github.com/kerberos-io/agent/machinery/docs"
	"github.com/kerberos-io/agent/machinery/src/models"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// @title Swagger Kerberos Agent API
// @version 1.0
// @description This is the API for using and configure Kerberos Agent.
// @termsOfService https://kerberos.io

// @contact.name API Support
// @contact.url https://www.kerberos.io
// @contact.email support@kerberos.io

// @license.name Apache 2.0 - Commons Clause
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html

// @BasePath /

// @securityDefinitions.apikey Bearer
// @in header
// @name Authorization

func StartServer(configuration *models.Configuration, communication *models.Communication) {

	// Initialize REST API
	r := gin.Default()

	// Profileerggerg
	pprof.Register(r)

	// Setup CORS
	r.Use(CORS())

	// Add Swagger
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// The JWT middlewareergreggre
	middleWare := JWTMiddleWare()
	authMiddleware, err := jwt.New(&middleWare)
	if err != nil {
		log.Fatal("JWT Error:" + err.Error())
	}

	// Add all routes
	AddRoutes(r, authMiddleware, configuration, communication)

	// Add static routes to UI
	r.Use(static.Serve("/", static.LocalFile("./www", true)))
	r.Use(static.Serve("/dashboard", static.LocalFile("./www", true)))
	r.Use(static.Serve("/media", static.LocalFile("./www", true)))
	r.Use(static.Serve("/settings", static.LocalFile("./www", true)))
	r.Use(static.Serve("/login", static.LocalFile("./www", true)))
	r.StaticFS("/file", gin.Dir("./data/recordings", false))

	// Run the api on port
	err = r.Run(":" + configuration.Port)
	if err != nil {
		log.Fatal(err)
	}
}
