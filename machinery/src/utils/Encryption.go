package utils

import (
	"github.com/forgoer/openssl"
)

type Crypter struct {
	key []byte
	iv  []byte
}

func NewCrypter(key []byte, iv []byte) (*Crypter, error) {
	return &Crypter{key: key, iv: iv}, nil
}

func (c *Crypter) Encrypt(data []byte) ([]byte, error) {
	return openssl.AesCBCEncrypt(data, c.key, c.iv, openssl.PKCS7_PADDING)
}

func (c *Crypter) Decrypt(data []byte) ([]byte, error) {
	return openssl.AesCBCDecrypt(data, c.key, c.iv, openssl.PKCS7_PADDING)
}

func (c *Crypter) EncryptECB(data []byte) ([]byte, error) {
	return openssl.AesECBEncrypt(data, c.key, openssl.PKCS7_PADDING)
}

func (c *Crypter) DecryptECB(data []byte) ([]byte, error) {
	return openssl.AesECBDecrypt(data, c.key, openssl.PKCS7_PADDING)
}
