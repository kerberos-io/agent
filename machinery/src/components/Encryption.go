package components

import (
	"crypto/aes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"

	"github.com/gin-gonic/gin"
)

// GenerateKeys godoc
// @Router /keys/generate [post]
// @ID generate-keys
// @Tags encryption
// @Summary GenerateKeys generates a public and private key pair, based on the RSA algorithm
// @Description GenerateKeys generates a public and private key pair, based on the RSA algorithm
// @Success 200 {object} models.APIResponse
func GenerateKeys(c *gin.Context) {

	// Create a new RSA key pair
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	privateKey := key
	publicKey := key.PublicKey

	// Export to PEM
	privateKeyPEM := string(pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}))

	publicKeyPEM := string(pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: x509.MarshalPKCS1PublicKey(&publicKey),
	}))

	// Create random string 32bytes
	randomKey := make([]byte, 32)
	_, err = rand.Read(randomKey)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	// Create AES symmetric key
	_, err = aes.NewCipher([]byte(randomKey))
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"public_key":  publicKeyPEM,
		"private_key": privateKeyPEM,
		"shared_key":  randomKey,
	})
}
