package encryption

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"testing"
	"time"
)

const testRecordingKey = "correct horse battery staple"

func TestEncryptRecordingRoundTrip(t *testing.T) {
	content := bytes.Repeat([]byte("mp4-bytes"), 1000)
	encrypted, err := EncryptRecording(content, testRecordingKey)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(encrypted, []byte(recordingMagic)) {
		t.Fatalf("missing format header: %q", encrypted[:8])
	}
	if got := binary.BigEndian.Uint32(encrypted[len(recordingMagic):]); got != recordingIterations {
		t.Fatalf("iterations in header = %d, want %d", got, recordingIterations)
	}
	decrypted, err := DecryptRecording(encrypted, testRecordingKey)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decrypted, content) {
		t.Fatal("round trip changed the content")
	}
}

// TestRecordingFormatMatchesSpecification decrypts with only the documented
// layout and the standard library, so the format is pinned independently of
// the implementation.
func TestRecordingFormatMatchesSpecification(t *testing.T) {
	encrypted, err := EncryptRecording([]byte("payload"), testRecordingKey)
	if err != nil {
		t.Fatal(err)
	}
	iterations := int(binary.BigEndian.Uint32(encrypted[8:12]))
	salt, nonce := encrypted[12:28], encrypted[28:40]
	key, err := pbkdf2.Key(sha256.New, testRecordingKey, salt, iterations, 32)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	plain, err := gcm.Open(nil, nonce, encrypted[40:], encrypted[:40])
	if err != nil || string(plain) != "payload" {
		t.Fatalf("independent PBKDF2-SHA256 + AES-256-GCM decryption failed: %q, %v", plain, err)
	}
}

func TestDecryptRecordingReadsLegacyFormat(t *testing.T) {
	legacy, err := AesEncrypt([]byte("old recording"), testRecordingKey)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecryptRecording(legacy, testRecordingKey)
	if err != nil || string(got) != "old recording" {
		t.Fatalf("legacy recording not readable: %q, %v", got, err)
	}
}

func TestEncryptRecordingUsesFreshNonces(t *testing.T) {
	a, _ := EncryptRecording([]byte("same"), testRecordingKey)
	b, _ := EncryptRecording([]byte("same"), testRecordingKey)
	if bytes.Equal(a[28:40], b[28:40]) {
		t.Fatal("two recordings share a nonce")
	}
	if bytes.Equal(a, b) {
		t.Fatal("identical plaintexts produced identical ciphertexts")
	}
}

func TestEncryptRecordingCachesDerivedKey(t *testing.T) {
	if _, err := EncryptRecording([]byte("warm"), testRecordingKey); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	for i := 0; i < 20; i++ {
		if _, err := EncryptRecording([]byte("frame"), testRecordingKey); err != nil {
			t.Fatal(err)
		}
	}
	// 20 fresh PBKDF2 derivations at 600k iterations would take seconds.
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("20 encryptions took %v; the derived key is not being reused", elapsed)
	}
}

func TestDecryptRecordingRejectsWrongKeyAndTampering(t *testing.T) {
	encrypted, err := EncryptRecording([]byte("secret footage"), testRecordingKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecryptRecording(encrypted, "wrong key"); !errors.Is(err, ErrRecordingDecrypt) {
		t.Fatalf("wrong key: err = %v", err)
	}

	payload := append([]byte(nil), encrypted...)
	payload[len(payload)-1] ^= 0x01
	if _, err := DecryptRecording(payload, testRecordingKey); !errors.Is(err, ErrRecordingDecrypt) {
		t.Fatalf("modified payload: err = %v", err)
	}

	header := append([]byte(nil), encrypted...)
	header[30] ^= 0x01 // inside the nonce, which is authenticated
	if _, err := DecryptRecording(header, testRecordingKey); !errors.Is(err, ErrRecordingDecrypt) {
		t.Fatalf("modified header: err = %v", err)
	}
}

func TestDecryptRecordingRejectsExcessiveIterations(t *testing.T) {
	encrypted, _ := EncryptRecording([]byte("x"), testRecordingKey)
	crafted := append([]byte(nil), encrypted...)
	binary.BigEndian.PutUint32(crafted[8:12], 0xFFFFFFFF)
	start := time.Now()
	if _, err := DecryptRecording(crafted, testRecordingKey); !errors.Is(err, ErrRecordingDecrypt) {
		t.Fatalf("crafted iteration count: err = %v", err)
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("crafted iteration count was not rejected before key derivation")
	}
}

func TestDecryptRecordingNeverPanicsOnMalformedInput(t *testing.T) {
	legacy, _ := AesEncrypt([]byte("old recording"), testRecordingKey)
	current, _ := EncryptRecording([]byte("new recording"), testRecordingKey)
	inputs := [][]byte{nil, {}, []byte("Salted__"), []byte("KRBREC1\x00"), []byte("not a recording")}
	for n := 0; n < len(legacy); n++ {
		inputs = append(inputs, legacy[:n])
	}
	for n := 0; n < 60 && n < len(current); n++ {
		inputs = append(inputs, current[:n])
	}
	for _, input := range inputs {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("DecryptRecording panicked on %d bytes: %v", len(input), r)
				}
			}()
			_, _ = DecryptRecording(input, testRecordingKey)
			_, _ = DecryptRecording(input, "wrong key")
		}()
	}
	if _, err := DecryptRecording([]byte("plain mp4"), testRecordingKey); !errors.Is(err, ErrRecordingFormat) {
		t.Fatalf("unknown format: err = %v", err)
	}
}

func TestLegacyDecryptWithWrongKeyReturnsError(t *testing.T) {
	legacy, _ := AesEncrypt([]byte("old recording"), testRecordingKey)
	// With the wrong key the padding is garbage; it used to be trusted and
	// could slice out of range.
	for i := 0; i < 50; i++ {
		if out, err := AesDecrypt(legacy, "wrong key "+string(rune('a'+i))); err == nil && string(out) == "old recording" {
			t.Fatal("wrong key decrypted the recording")
		}
	}
}
