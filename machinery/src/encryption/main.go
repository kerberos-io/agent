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

func DecryptWithPrivateKey(ciphertext string, privateKey *rsa.PrivateKey) ([]byte, error) {
	cipheredValue, _ := base64.StdEncoding.DecodeString(ciphertext)
	out, err := rsa.DecryptPKCS1v15(nil, privateKey, cipheredValue)
	return out, err
}

func SignWithPrivateKey(data []byte, privateKey *rsa.PrivateKey) ([]byte, error) {
	hashed := sha256.Sum256(data)
	signature, err := rsa.SignPKCS1v15(nil, privateKey, crypto.SHA256, hashed[:])
	return signature, err
}

func AesEncrypt(content []byte, password string) ([]byte, error) {
	salt := make([]byte, 8)
	_, err := rand.Read(salt)
	if err != nil {
		return nil, err
	}
	key, iv, err := DefaultEvpKDF([]byte(password), salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	mode := cipher.NewCBCEncrypter(block, iv)
	cipherBytes := PKCS5Padding(content, aes.BlockSize)
	mode.CryptBlocks(cipherBytes, cipherBytes)

	cipherText := make([]byte, 16+len(cipherBytes))
	copy(cipherText[:8], []byte("Salted__"))
	copy(cipherText[8:16], salt)
	copy(cipherText[16:], cipherBytes)
	return cipherText, nil
}

func AesDecrypt(cipherText []byte, password string) ([]byte, error) {
	if string(cipherText[:8]) != "Salted__" {
		return nil, errors.New("invalid crypto js aes encryption")
	}

	salt := cipherText[8:16]
	cipherBytes := cipherText[16:]
	key, iv, err := DefaultEvpKDF([]byte(password), salt)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(cipherBytes, cipherBytes)

	result := PKCS5UnPadding(cipherBytes)
	return result, nil
}

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
	keySize := 256 / 32
	ivSize := 128 / 32
	derivedKeyBytes, err := EvpKDF(password, salt, keySize+ivSize, 1, "md5")
	if err != nil {
		return []byte{}, []byte{}, err
	}
	return derivedKeyBytes[:keySize*4], derivedKeyBytes[keySize*4:], nil
}

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
