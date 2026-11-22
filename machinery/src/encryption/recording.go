package encryption

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"strconv"
	"sync"
)

// Recording encryption format, version 1:
//
//	magic       8 bytes  "KRBREC1\x00"
//	iterations  4 bytes  big-endian PBKDF2-HMAC-SHA256 iteration count
//	salt       16 bytes
//	nonce      12 bytes
//	payload     AES-256-GCM ciphertext followed by the 16-byte tag
//
// The header (magic through nonce) is authenticated as additional data, so
// any change to it or to the payload is detected on decryption.
//
// Recordings encrypted by older releases use the OpenSSL/CryptoJS "Salted__"
// format (a single MD5 round to derive the key). DecryptRecording still reads
// those, so existing recordings remain playable.
const (
	recordingMagic          = "KRBREC1\x00"
	recordingSaltSize       = 16
	recordingNonceSize      = 12
	recordingHeaderSize     = len(recordingMagic) + 4 + recordingSaltSize + recordingNonceSize
	recordingKeySize        = 32
	recordingIterations     = 600_000
	recordingMaxIterations  = 10_000_000
	recordingKeyCacheLimit  = 16
	legacyOpenSSLSaltHeader = "Salted__"
)

var (
	ErrRecordingFormat    = errors.New("unknown recording encryption format")
	ErrRecordingDecrypt   = errors.New("recording could not be decrypted: wrong key or corrupted file")
	errLegacyCiphertext   = errors.New("invalid legacy ciphertext")
	recordingKeyCacheMu   sync.Mutex
	recordingKeyCache     = map[string][]byte{}
	recordingSaltOnce     sync.Once
	recordingProcessSalt  []byte
	recordingProcessError error
)

// processSalt returns a random salt generated once per process. Deriving the
// key with PBKDF2 is deliberately slow, so encryption reuses one salt and the
// cached key for every recording instead of paying that cost per file. Each
// recording still gets a unique random nonce.
func processSalt() ([]byte, error) {
	recordingSaltOnce.Do(func() {
		salt := make([]byte, recordingSaltSize)
		if _, err := rand.Read(salt); err != nil {
			recordingProcessError = err
			return
		}
		recordingProcessSalt = salt
	})
	return recordingProcessSalt, recordingProcessError
}

func recordingKey(password string, salt []byte, iterations int) ([]byte, error) {
	cacheKey := string(salt) + "\x00" + strconv.Itoa(iterations) + "\x00" + password
	recordingKeyCacheMu.Lock()
	if key, ok := recordingKeyCache[cacheKey]; ok {
		recordingKeyCacheMu.Unlock()
		return key, nil
	}
	recordingKeyCacheMu.Unlock()

	key, err := pbkdf2.Key(sha256.New, password, salt, iterations, recordingKeySize)
	if err != nil {
		return nil, err
	}

	recordingKeyCacheMu.Lock()
	if len(recordingKeyCache) >= recordingKeyCacheLimit {
		recordingKeyCache = map[string][]byte{}
	}
	recordingKeyCache[cacheKey] = key
	recordingKeyCacheMu.Unlock()
	return key, nil
}

// EncryptRecording encrypts a recording with a key derived from password.
func EncryptRecording(content []byte, password string) ([]byte, error) {
	if password == "" {
		return nil, errors.New("empty encryption key")
	}
	salt, err := processSalt()
	if err != nil {
		return nil, err
	}
	key, err := recordingKey(password, salt, recordingIterations)
	if err != nil {
		return nil, err
	}
	gcm, err := newRecordingGCM(key)
	if err != nil {
		return nil, err
	}

	out := make([]byte, recordingHeaderSize, recordingHeaderSize+len(content)+gcm.Overhead())
	copy(out, recordingMagic)
	binary.BigEndian.PutUint32(out[len(recordingMagic):], uint32(recordingIterations))
	copy(out[len(recordingMagic)+4:], salt)
	nonce := out[len(recordingMagic)+4+recordingSaltSize : recordingHeaderSize]
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	header := out[:recordingHeaderSize]
	return gcm.Seal(out, nonce, content, header), nil
}

// DecryptRecording decrypts a recording written by EncryptRecording, or by
// older releases in the legacy OpenSSL "Salted__" format.
func DecryptRecording(data []byte, password string) ([]byte, error) {
	switch {
	case bytes.HasPrefix(data, []byte(recordingMagic)):
		return decryptRecordingV1(data, password)
	case bytes.HasPrefix(data, []byte(legacyOpenSSLSaltHeader)):
		return AesDecrypt(data, password)
	default:
		return nil, ErrRecordingFormat
	}
}

func decryptRecordingV1(data []byte, password string) ([]byte, error) {
	if len(data) < recordingHeaderSize+16 {
		return nil, ErrRecordingDecrypt
	}
	iterations := int(binary.BigEndian.Uint32(data[len(recordingMagic):]))
	if iterations < 1 || iterations > recordingMaxIterations {
		return nil, ErrRecordingDecrypt
	}
	salt := data[len(recordingMagic)+4 : len(recordingMagic)+4+recordingSaltSize]
	nonce := data[len(recordingMagic)+4+recordingSaltSize : recordingHeaderSize]

	key, err := recordingKey(password, salt, iterations)
	if err != nil {
		return nil, err
	}
	gcm, err := newRecordingGCM(key)
	if err != nil {
		return nil, err
	}
	plain, err := gcm.Open(nil, nonce, data[recordingHeaderSize:], data[:recordingHeaderSize])
	if err != nil {
		return nil, ErrRecordingDecrypt
	}
	return plain, nil
}

func newRecordingGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
