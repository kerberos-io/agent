package models

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPackageMQTTMessageCachesCurrentSigningKey(t *testing.T) {
	resetMQTTPrivateKeyCache(t)

	privateKey := generatePKCS8RSAPrivateKeyPEM(t)

	var parseCalls atomic.Int32
	restore := hookPKCS8PrivateKeyParser(t, func(der []byte) (any, error) {
		parseCalls.Add(1)
		return x509.ParsePKCS8PrivateKey(der)
	})
	defer restore()

	msg := Message{
		Payload: Payload{
			DeviceId: "device-1",
			Value: map[string]interface{}{
				"hello": "world",
			},
		},
	}

	first, err := PackageMQTTMessage(encryptedConfiguration(privateKey), msg)
	if err != nil {
		t.Fatalf("first PackageMQTTMessage() error = %v", err)
	}
	assertEncryptedMQTTMessage(t, first)

	second, err := PackageMQTTMessage(encryptedConfiguration(privateKey), msg)
	if err != nil {
		t.Fatalf("second PackageMQTTMessage() error = %v", err)
	}
	assertEncryptedMQTTMessage(t, second)

	if got := parseCalls.Load(); got != 1 {
		t.Fatalf("cached parse calls = %d, want 1", got)
	}
}

func TestPackageMQTTMessageReplacesCachedSigningKeyOnRotation(t *testing.T) {
	resetMQTTPrivateKeyCache(t)

	privateKey := generatePKCS8RSAPrivateKeyPEM(t)
	rotatedPrivateKey := generatePKCS8RSAPrivateKeyPEM(t)

	var parseCalls atomic.Int32
	restore := hookPKCS8PrivateKeyParser(t, func(der []byte) (any, error) {
		parseCalls.Add(1)
		return x509.ParsePKCS8PrivateKey(der)
	})
	defer restore()

	msg := Message{
		Payload: Payload{
			DeviceId: "device-1",
			Value: map[string]interface{}{
				"hello": "world",
			},
		},
	}

	for _, key := range []string{privateKey, rotatedPrivateKey, privateKey} {
		payload, err := PackageMQTTMessage(encryptedConfiguration(key), msg)
		if err != nil {
			t.Fatalf("PackageMQTTMessage() error = %v", err)
		}
		assertEncryptedMQTTMessage(t, payload)
	}

	if got := parseCalls.Load(); got != 3 {
		t.Fatalf("parse calls across old/new/old rotation = %d, want 3", got)
	}
}

func TestPackageMQTTMessageConcurrentCallsReuseCachedSigningKey(t *testing.T) {
	resetMQTTPrivateKeyCache(t)

	privateKey := generatePKCS8RSAPrivateKeyPEM(t)

	var parseCalls atomic.Int32
	restore := hookPKCS8PrivateKeyParser(t, func(der []byte) (any, error) {
		parseCalls.Add(1)
		time.Sleep(10 * time.Millisecond)
		return x509.ParsePKCS8PrivateKey(der)
	})
	defer restore()

	msg := Message{
		Payload: Payload{
			DeviceId: "device-1",
			Value: map[string]interface{}{
				"hello": "world",
			},
		},
	}

	start := make(chan struct{})
	var workers sync.WaitGroup
	for i := 0; i < 16; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start

			payload, err := PackageMQTTMessage(encryptedConfiguration(privateKey), msg)
			if err != nil {
				t.Errorf("PackageMQTTMessage() error = %v", err)
				return
			}
			assertEncryptedMQTTMessage(t, payload)
		}()
	}

	close(start)
	workers.Wait()

	if got := parseCalls.Load(); got != 1 {
		t.Fatalf("concurrent parse calls = %d, want 1", got)
	}
}

func encryptedConfiguration(privateKey string) *Configuration {
	return &Configuration{
		Config: Config{
			Encryption: &Encryption{
				Enabled:      "true",
				PrivateKey:   privateKey,
				SymmetricKey: "secret",
			},
		},
	}
}

func generatePKCS8RSAPrivateKeyPEM(t *testing.T) string {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() error = %v", err)
	}

	der, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatalf("x509.MarshalPKCS8PrivateKey() error = %v", err)
	}

	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func assertEncryptedMQTTMessage(t *testing.T, payload []byte) {
	t.Helper()

	var got Message
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if !got.Encrypted {
		t.Fatal("message is not marked encrypted")
	}
	if got.Payload.EncryptedValue == "" {
		t.Fatal("encrypted value is empty")
	}
	if got.Payload.Signature == "" {
		t.Fatal("signature is empty")
	}
	if len(got.Payload.Value) != 0 {
		t.Fatalf("payload value = %#v, want cleared map", got.Payload.Value)
	}
}

func hookPKCS8PrivateKeyParser(t *testing.T, parser func([]byte) (any, error)) func() {
	t.Helper()

	original := parsePKCS8PrivateKey
	parsePKCS8PrivateKey = parser

	return func() {
		parsePKCS8PrivateKey = original
	}
}

func resetMQTTPrivateKeyCache(t *testing.T) {
	t.Helper()
	rsaPrivateKeyCache = newRSAPrivateKeyCache()
}
