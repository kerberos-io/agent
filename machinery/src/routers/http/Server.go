package http

import (
	"io"
	"os"
	"strconv"

	jwt "github.com/appleboy/gin-jwt/v2"
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/contrib/static"
	"github.com/gin-gonic/gin"

	//Swagger documentantion
	"log"

	_ "github.com/kerberos-io/agent/machinery/docs"
	"github.com/kerberos-io/agent/machinery/src/capture"
	"github.com/kerberos-io/agent/machinery/src/encryption"
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

func StartServer(configDirectory string, configuration *models.Configuration, communication *models.Communication, captureDevice *capture.Capture) {

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
	AddRoutes(r, authMiddleware, configDirectory, configuration, communication, captureDevice)

	// Update environment variables
	environmentVariables := configDirectory + "/www/env.js"
	if os.Getenv("AGENT_MODE") == "demo" {
		demoEnvironmentVariables := configDirectory + "/www/env.demo.js"
		// Move demo environment variables to environment variables
		err := os.Rename(demoEnvironmentVariables, environmentVariables)
		if err != nil {
			log.Fatal(err)
		}
	}

	// Add static routes to UI
	r.Use(static.Serve("/", static.LocalFile(configDirectory+"/www", true)))
	r.Use(static.Serve("/dashboard", static.LocalFile(configDirectory+"/www", true)))
	r.Use(static.Serve("/media", static.LocalFile(configDirectory+"/www", true)))
	r.Use(static.Serve("/settings", static.LocalFile(configDirectory+"/www", true)))
	r.Use(static.Serve("/login", static.LocalFile(configDirectory+"/www", true)))
	r.Handle("GET", "/file/*filepath", func(c *gin.Context) {
		Files(c, configDirectory, configuration)
	})

	// Run the api on port
	err = r.Run(":" + configuration.Port)
	if err != nil {
		log.Fatal(err)
	}
}

func Files(c *gin.Context, configDirectory string, configuration *models.Configuration) {

	// Get File
	filePath := configDirectory + "/data/recordings" + c.Param("filepath")
	_, err := os.Open(filePath)
	if err != nil {
		c.JSON(404, gin.H{"error": "File not found"})
		return
	}

	contents, err := os.ReadFile(filePath)
	if err == nil {

		// Get symmetric key
		symmetricKey := configuration.Config.Encryption.SymmetricKey
		encryptedRecordings := configuration.Config.Encryption.Recordings
		// Decrypt file
		if encryptedRecordings == "true" && symmetricKey != "" {

			// Read file
			if err != nil {
				c.JSON(404, gin.H{"error": "File not found"})
				return
			}

			// Decrypt file
			contents, err = encryption.AesDecrypt(contents, symmetricKey)
			if err != nil {
				c.JSON(404, gin.H{"error": "File not found"})
				return
			}
		}

		// Get fileSize from contents
		fileSize := len(contents)

		// Send file to gin
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Content-Disposition", "attachment; filename="+filePath)
		c.Header("Content-Type", "video/mp4")
		c.Header("Content-Length", strconv.Itoa(fileSize))
		// Send contents to gin
		io.WriteString(c.Writer, string(contents))
	} else {
		c.JSON(404, gin.H{"error": "File not found"})
		return
	}
}
