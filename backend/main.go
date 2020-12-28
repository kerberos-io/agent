package main

import (
	"fmt"
	"github.com/appleboy/gin-jwt/v2"
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/contrib/static"
	"github.com/gin-gonic/gin"
	"github.com/kerberos-io/opensource/backend/src/routers/http"
	"log"
	"os"
)

func main(){

	const VERSION = 3.0
	action := os.Args[1]

	switch action {

		case "version":

			log.Printf("%s: %0.1f\n", "You are currrently running Kerberos Open Source", VERSION)

		case "pending-upload":

			name := os.Args[2]
      		fmt.Println(name)
			//cloud.PendingUpload(log, name)

		case "discover":

			timeout := os.Args[2]
      		fmt.Println(timeout)
			//duration, _ := strconv.Atoi(timeout)
			//capture.Discover(log, time.Duration(duration))

		case "run": {

			name := os.Args[2]
			port := os.Args[3]
			fmt.Println(name)

			// Initialize REST API
			r := gin.Default()

			// Profile
  			pprof.Register(r)

			// Setup CORS
			r.Use(http.CORS())

			// Serve frontend static files
			r.Use(static.Serve("/", static.LocalFile("./www", true)))

			// The JWT middleware
			middleWare := http.JWTMiddleWare()
			authMiddleware, err := jwt.New(&middleWare)
			if err != nil {
				log.Fatal("JWT Error:" + err.Error())
			}

			api := r.Group("/api")
			{
				api.GET("/configure", func(c *gin.Context) {
					c.JSON(200, gin.H{
						"restart": true,
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
				})

				api.Use(authMiddleware.MiddlewareFunc())
				{

				}
			}

			yolo := "ok"

			r.Run(":" + port)

		}
		default:
			fmt.Println("Sorry I don't understand :(")
	}
	return
}
