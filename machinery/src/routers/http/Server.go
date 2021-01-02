package http

import (
	"github.com/appleboy/gin-jwt/v2"
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/contrib/static"
	"github.com/gin-gonic/gin"
	_ "github.com/kerberos-io/opensource/machinery/docs"
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

func StartServer(name string, port string) {

	// Initialize REST API
	r := gin.Default()

	// Profile
	pprof.Register(r)

	// Setup CORS
	r.Use(CORS())

	// Serve web static files
	r.Use(static.Serve("/", static.LocalFile("./www", true)))

	// Add Swagger
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// The JWT middleware
	middleWare := JWTMiddleWare()
	authMiddleware, err := jwt.New(&middleWare)
	if err != nil {
		log.Fatal("JWT Error:" + err.Error())
	}

	// Add all routes
	AddRoutes(r, authMiddleware)

	// Run the api on port
	r.Run(":" + port)
}
