package http

import (
	"net/http"
	"os"
	"time"

	jwt "github.com/appleboy/gin-jwt/v2"
	"github.com/gin-gonic/gin"
	jwtgo "github.com/golang-jwt/jwt/v4"
	"github.com/kerberos-io/agent/machinery/src/models"
)

func JWTMiddleWare(configDirectory string) jwt.GinJWTMiddleware {

	identityKey := "id"
	// AGENT_JWT_SECRET lets tokens issued by another service (e.g. the
	// Kerberos Factory) be validated by the agent. Without it, a random secret
	// is generated once and stored with the configuration; see loadJWTSecret.
	myKey := loadJWTSecret(configDirectory)
	// With an explicitly shared secret, Kerberos Factory signs its own tokens
	// for the agent under the "factory" service identity. Only holders of that
	// secret can produce them. With the generated per-install secret, only the
	// agent's own login issues tokens, so that identity is not accepted.
	sharedSecret := os.Getenv("AGENT_JWT_SECRET") != ""

	m := jwt.GinJWTMiddleware{
		Realm:       "kerberosio",
		Key:         myKey,
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
			user, ok := claims["id"].(map[string]interface{})
			if !ok {
				return nil
			}
			username, _ := user["username"].(string)
			role, _ := user["role"].(string)
			return &models.User{
				Username: username,
				Role:     role,
			}
		},
		Authenticator: func(c *gin.Context) (interface{}, error) {
			var loginVals models.User
			if err := c.ShouldBind(&loginVals); err != nil {
				return "", jwt.ErrMissingLoginValues
			}
			username := loginVals.Username
			password := loginVals.Password

			usernameFromConfig := agentUsername()
			// Get password from ENV
			passwordFromConfig := os.Getenv("AGENT_PASSWORD")
			if passwordFromConfig == "" {
				passwordFromConfig = "root"
			}

			if username == usernameFromConfig && password == passwordFromConfig {
				return &models.User{
					Username: username,
					Role:     "admin",
				}, nil
			} else {
				return nil, jwt.ErrFailedAuthentication
			}
		},
		LoginResponse: func(c *gin.Context, code int, token string, expire time.Time) {

			// Decrypt the token
			hmacSecret := myKey
			t, _ := jwtgo.Parse(token, func(token *jwtgo.Token) (interface{}, error) {
				return hmacSecret, nil
			})

			// Get the claims
			claims, _ := t.Claims.(jwtgo.MapClaims)
			user := claims["id"].(map[string]interface{})

			c.JSON(http.StatusOK, gin.H{
				"code":     http.StatusOK,
				"token":    token,
				"expire":   expire.Format(time.RFC3339),
				"username": user["username"].(string),
				"role":     user["role"].(string),
			})
		},
		Authorizator: func(data interface{}, c *gin.Context) bool {
			// Only the configured account may use the API, even with a
			// validly signed token.
			user, ok := data.(*models.User)
			if !ok || user.Username == "" {
				return false
			}
			return user.Username == agentUsername() || (sharedSecret && user.Username == factoryServiceUsername)
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
		// X-Authorization is included because requests proxied through the
		// Kubernetes apiserver service-proxy have their Authorization header
		// consumed by the apiserver; the original bearer token is forwarded in
		// the X-Authorization header instead.
		TokenLookup: "header: Authorization, header: X-Authorization, query: token, cookie: jwt",
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
