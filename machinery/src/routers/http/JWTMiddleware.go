package http

import (
	jwt "github.com/appleboy/gin-jwt/v2"
	jwtgo "github.com/dgrijalva/jwt-go"
	"github.com/gin-gonic/gin"
	"github.com/kerberos-io/opensource/machinery/src/models"
	"net/http"
	"time"
)

func JWTMiddleWare() jwt.GinJWTMiddleware {

	identityKey := "id"
	myKey := "TOBECHANGED"

	m := jwt.GinJWTMiddleware{
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
			user := claims["id"].(map[string]interface{})
			return &models.User{
				Username: user["username"].(string),
				Role:     user["role"].(string),
			}
		},
		Authenticator: func(c *gin.Context) (interface{}, error) {
			var loginVals models.User
			if err := c.ShouldBind(&loginVals); err != nil {
				return "", jwt.ErrMissingLoginValues
			}
			username := loginVals.Username
			password := loginVals.Password

			usernameENV := "root"
			passwordENV := "root"
			if username == usernameENV && password == passwordENV {
				return &models.User{
					Username:  username,
					Role: "admin",
				}, nil
			} else {
				return nil, jwt.ErrFailedAuthentication
			}
		},
		LoginResponse: func(c *gin.Context, code int, token string, expire time.Time) {

			// Decrypt the token
			hmacSecret := []byte(myKey) // todo in config file
			t, _ := jwtgo.Parse(token, func(token *jwtgo.Token) (interface{}, error) {
				return hmacSecret, nil
			})

			// Get the claims
			claims, _ := t.Claims.(jwtgo.MapClaims)
			user := claims["id"].(map[string]interface {})

			c.JSON(http.StatusOK, gin.H{
				"code":   http.StatusOK,
				"token":  token,
				"expire": expire.Format(time.RFC3339),
				"username": user["username"].(string),
				"role": user["role"].(string),
			})
		},
		Authorizator: func(data interface{}, c *gin.Context) bool {
			if _, ok := data.(*models.User); ok { //&& v.Username == "admin" {
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

		// TimeFunc provides the current time. You can override it to use another time
		// value. This is useful for testing or if your server uses a different time zone than your tokens.
		TimeFunc: time.Now,
	}
	return m
}
