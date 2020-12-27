package main

import (
	"fmt"
	"github.com/appleboy/gin-jwt/v2"
	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/contrib/static"
	"github.com/gin-gonic/gin"
	"github.com/kerberos-io/opensource/backend/src/models"
	"log"
	"os"
	"time"
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
			r.Use(cors.New(cors.Config{
				AllowOrigins:     []string{"*"},
				AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE"},
				AllowHeaders:     []string{"Origin", "Content-Type", "authorization", "multipart/form-data", "x-requested-with"},
				ExposeHeaders:    []string{"Content-Length"},
				AllowCredentials: true,
				MaxAge: 12 * time.Hour,
			}))

			// Serve frontend static files
			r.Use(static.Serve("/", static.LocalFile("./www", true)))

			// the jwt middleware
		  	identityKey := "id"
		  	myKey := "TOBECHANGED"
			authMiddleware, err := jwt.New(&jwt.GinJWTMiddleware{
				Realm:       "kerberosio",
				Key:         []byte(myKey),
				Timeout:     time.Hour * 24,
				MaxRefresh:  time.Hour * 24 * 7,
				IdentityKey: identityKey,
				PayloadFunc: func(data interface{}) jwt.MapClaims {
					if v, ok := data.(*models.User); ok {
						return jwt.MapClaims{
							identityKey: v,
						}
					}
					return jwt.MapClaims{}
				},
				IdentityHandler: func(c *gin.Context) interface{} {
					claims := jwt.ExtractClaims(c)
					user := claims["id"].(map[string]interface {})
					return &models.User{
						Username: user["username"].(string),
						Role: user["role"].(string),
					}
				},
				Authorizator: func(data interface{}, c *gin.Context) bool {
					if _, ok := data.(*models.User); ok  { //&& v.Username == "admin" {
						return true
					}
					return false
				},
				Unauthorized: func(c *gin.Context, code int, message string) {
					c.AbortWithStatusJSON(code, gin.H{
						"code":    code,
						"message": message,
					})
				},
				// TokenLookup is a string in the form of "<source>:<name>" that is used
				// to extract token from the request.
				// Optional. Default value "header:Authorization".
				// Possible values:
				// - "header:<name>"
				// - "query:<name>"
				// - "cookie:<name>"
				// - "param:<name>"
				TokenLookup: "header: Authorization, query: token, cookie: jwt",
				// TokenLookup: "query:token",
				// TokenLookup: "cookie:token",

				// TokenHeadName is a string in the header. Default value is "Bearer"
				TokenHeadName: "Bearer",

				// TimeFunc provides the current time. You can override it to use another time value. This is useful for testing or if your server uses a different time zone than your tokens.
				TimeFunc: time.Now,
			})

			if err != nil {
				log.Fatal("JWT Error:" + err.Error())
			}

			api := r.Group("/api")
			{
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

			r.Run(":" + port)

		}
		default:
			fmt.Println("Sorry I don't understand :(")
	}
	return
}
