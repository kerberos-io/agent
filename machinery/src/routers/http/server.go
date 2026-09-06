package http

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	jwt "github.com/appleboy/gin-jwt/v2"
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/contrib/static"
	"github.com/gin-gonic/gin"

	//Swagger documentantion
	log "github.com/sirupsen/logrus"

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

	// Set release mode
	gin.SetMode(gin.ReleaseMode)

	// Initialize REST API
	r := gin.New()
	r.Use(requestLogger(), gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		log.WithFields(log.Fields{
			"component": "http",
			"event":     "request_panic",
			"method":    c.Request.Method,
			"path":      c.Request.URL.Path,
			"panic":     fmt.Sprint(recovered),
		}).Error("HTTP request panicked")
		c.AbortWithStatus(500)
	}))

	// Profiler
	pprof.Register(r)

	// Setup CORS
	r.Use(CORS())

	// Add Swagger
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// The JWT middlewareergreggre
	middleWare := JWTMiddleWare()
	authMiddleware, err := jwt.New(&middleWare)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"component": "http",
			"event":     "jwt_initialization_failed",
		}).Fatal("Failed to initialize JWT middleware")
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
			log.WithError(err).WithFields(log.Fields{
				"component": "http",
				"event":     "demo_environment_move_failed",
			}).Fatal("Failed to activate demo UI environment")
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
	log.WithFields(log.Fields{
		"component": "http",
		"event":     "server_starting",
		"port":      configuration.Port,
	}).Info("HTTP server starting")
	err = r.Run(":" + configuration.Port)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"component": "http",
			"event":     "server_failed",
			"port":      configuration.Port,
		}).Fatal("HTTP server stopped")
	}
}

func requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		startedAt := time.Now()
		c.Next()

		fields := log.Fields{
			"component":      "http",
			"duration_ms":    time.Since(startedAt).Milliseconds(),
			"event":          "request_completed",
			"method":         c.Request.Method,
			"path":           c.Request.URL.Path,
			"response_bytes": c.Writer.Size(),
			"status":         c.Writer.Status(),
		}
		if log.IsLevelEnabled(log.DebugLevel) {
			fields["client_ip"] = c.ClientIP()
			fields["user_agent"] = c.Request.UserAgent()
		}
		entry := log.WithFields(fields)
		if len(c.Errors) > 0 {
			entry = entry.WithField("request_errors", c.Errors.String())
		}

		switch status := c.Writer.Status(); {
		case status >= 500:
			entry.Error("HTTP request completed")
		case status >= 400:
			entry.Warn("HTTP request completed")
		default:
			entry.Info("HTTP request completed")
		}
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
