package encryption

import (
	"bytes"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"hash"
)

// DecryptWithPrivateKey decrypts data with private key
func DecryptWithPrivateKey(ciphertext string, privateKey *rsa.PrivateKey) ([]byte, error) {

	// decode our encrypted string into cipher bytes
	cipheredValue, _ := base64.StdEncoding.DecodeString(ciphertext)

	// decrypt the data
	out, err := rsa.DecryptPKCS1v15(nil, privateKey, cipheredValue)

	return out, err
}

// SignWithPrivateKey signs data with private key
func SignWithPrivateKey(data []byte, privateKey *rsa.PrivateKey) ([]byte, error) {

	// hash the data with sha256
	hashed := sha256.Sum256(data)

	// sign the data
	signature, err := rsa.SignPKCS1v15(nil, privateKey, crypto.SHA256, hashed[:])

	return signature, err
}

func AesEncrypt(content string, password string) (string, error) {
	salt := make([]byte, 8)
	_, err := rand.Read(salt)
	if err != nil {
		return "", err
	}
	key, iv, err := DefaultEvpKDF([]byte(password), salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	mode := cipher.NewCBCEncrypter(block, iv)
	cipherBytes := PKCS5Padding([]byte(content), aes.BlockSize)
	mode.CryptBlocks(cipherBytes, cipherBytes)

	data := make([]byte, 16+len(cipherBytes))
	copy(data[:8], []byte("Salted__"))
	copy(data[8:16], salt)
	copy(data[16:], cipherBytes)

	cipherText := base64.StdEncoding.EncodeToString(data)
	return cipherText, nil
}

func AesDecrypt(cipherText string, password string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(cipherText)
	if err != nil {
		return "", err
	}
	if string(data[:8]) != "Salted__" {
		return "", errors.New("invalid crypto js aes encryption")
	}

	salt := data[8:16]
	cipherBytes := data[16:]
	key, iv, err := DefaultEvpKDF([]byte(password), salt)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(cipherBytes, cipherBytes)

	result := PKCS5UnPadding(cipherBytes)
	return string(result), nil
}

// https://stackoverflow.com/questions/27677236/encryption-in-javascript-and-decryption-with-php/27678978#27678978
// https://github.com/brix/crypto-js/blob/8e6d15bf2e26d6ff0af5277df2604ca12b60a718/src/evpkdf.js#L55
func EvpKDF(password []byte, salt []byte, keySize int, iterations int, hashAlgorithm string) ([]byte, error) {
	var block []byte
	var hasher hash.Hash
	derivedKeyBytes := make([]byte, 0)
	switch hashAlgorithm {
	case "md5":
		hasher = md5.New()
	default:
		return []byte{}, errors.New("not implement hasher algorithm")
	}
	for len(derivedKeyBytes) < keySize*4 {
		if len(block) > 0 {
			hasher.Write(block)
		}
		hasher.Write(password)
		hasher.Write(salt)
		block = hasher.Sum([]byte{})
		hasher.Reset()

		for i := 1; i < iterations; i++ {
			hasher.Write(block)
			block = hasher.Sum([]byte{})
			hasher.Reset()
		}
		derivedKeyBytes = append(derivedKeyBytes, block...)
	}
	return derivedKeyBytes[:keySize*4], nil
}

func DefaultEvpKDF(password []byte, salt []byte) (key []byte, iv []byte, err error) {
	// https://github.com/brix/crypto-js/blob/8e6d15bf2e26d6ff0af5277df2604ca12b60a718/src/cipher-core.js#L775
	keySize := 256 / 32
	ivSize := 128 / 32
	derivedKeyBytes, err := EvpKDF(password, salt, keySize+ivSize, 1, "md5")
	if err != nil {
		return []byte{}, []byte{}, err
	}
	return derivedKeyBytes[:keySize*4], derivedKeyBytes[keySize*4:], nil
}

// https://stackoverflow.com/questions/41579325/golang-how-do-i-decrypt-with-des-cbc-and-pkcs7
func PKCS5UnPadding(src []byte) []byte {
	length := len(src)
	unpadding := int(src[length-1])
	return src[:(length - unpadding)]
}

func PKCS5Padding(src []byte, blockSize int) []byte {
	padding := blockSize - len(src)%blockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(src, padtext...)
}
