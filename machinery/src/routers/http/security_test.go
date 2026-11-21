package http

import (
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	jwt "github.com/appleboy/gin-jwt/v2"
	"github.com/gin-gonic/gin"
	jwtgo "github.com/golang-jwt/jwt/v4"
	"github.com/kerberos-io/agent/machinery/src/models"
)

func TestLoadJWTSecretPrefersEnvironment(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENT_JWT_SECRET", "shared-with-factory")

	if got := string(loadJWTSecret(dir)); got != "shared-with-factory" {
		t.Fatalf("loadJWTSecret() = %q, want the AGENT_JWT_SECRET value", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "data", "config", jwtSecretFileName)); !os.IsNotExist(err) {
		t.Fatalf("no secret file should be written when AGENT_JWT_SECRET is set, stat err = %v", err)
	}
}

func TestLoadJWTSecretGeneratesAndPersistsRandomSecret(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENT_JWT_SECRET", "")

	first := string(loadJWTSecret(dir))
	if first == legacyJWTSecret || len(first) != 64 {
		t.Fatalf("generated secret = %q, want 64 random hex characters", first)
	}

	path := filepath.Join(dir, "data", "config", jwtSecretFileName)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("secret file not written: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("secret file permissions = %o, want 600", perm)
	}

	if second := string(loadJWTSecret(dir)); second != first {
		t.Fatalf("secret changed across restarts: %q then %q", first, second)
	}
	if other := string(loadJWTSecret(t.TempDir())); other == first {
		t.Fatal("two installs generated the same secret")
	}
}

func TestLoadJWTSecretReplacesShortStoredSecret(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENT_JWT_SECRET", "")
	path := filepath.Join(dir, "data", "config", jwtSecretFileName)
	if err := writeJWTSecret(path, legacyJWTSecret); err != nil {
		t.Fatal(err)
	}

	got := string(loadJWTSecret(dir))
	if got == legacyJWTSecret || len(got) != 64 {
		t.Fatalf("short stored secret was reused: %q", got)
	}
}

// securityRouter wires the real middleware and routes the way StartServer does.
func securityRouter(t *testing.T) (*gin.Engine, []byte) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	t.Setenv("AGENT_JWT_SECRET", "")
	t.Setenv("AGENT_USERNAME", "")
	t.Setenv("AGENT_PASSWORD", "")

	dir := t.TempDir()
	config := JWTMiddleWare(dir)
	authMiddleware, err := jwt.New(&config)
	if err != nil {
		t.Fatalf("initialize JWT middleware: %v", err)
	}

	router := gin.New()
	router.Use(gin.Recovery())
	registerProfiler(router, authMiddleware)
	AddRoutes(router, authMiddleware, dir, &models.Configuration{}, &models.Communication{}, nil)
	registerFileRoute(router, authMiddleware, dir, &models.Configuration{})
	return router, config.Key
}

func signedToken(t *testing.T, key []byte, username string) string {
	t.Helper()
	token := jwtgo.NewWithClaims(jwtgo.SigningMethodHS256, jwtgo.MapClaims{
		"id":       map[string]interface{}{"username": username, "role": "admin"},
		"exp":      time.Now().Add(time.Hour).Unix(),
		"orig_iat": time.Now().Unix(),
	})
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func request(router *gin.Engine, method, target, bearer string) int {
	req := httptest.NewRequest(method, target, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec.Code
}

func TestAPIRejectsTokensSignedWithLegacyDefaultKey(t *testing.T) {
	router, _ := securityRouter(t)
	forged := signedToken(t, []byte(legacyJWTSecret), "root")

	if code := request(router, "GET", "/api/config", forged); code != stdhttp.StatusUnauthorized {
		t.Fatalf("token signed with the legacy key: status %d, want 401", code)
	}
}

func TestAPIRejectsTokensForOtherUsers(t *testing.T) {
	router, key := securityRouter(t)
	other := signedToken(t, key, "intruder")

	if code := request(router, "GET", "/api/config", other); code != stdhttp.StatusForbidden {
		t.Fatalf("validly signed token for another user: status %d, want 403", code)
	}
}

func TestLoginTokenGrantsAccess(t *testing.T) {
	router, _ := securityRouter(t)

	req := httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"username":"root","password":"root"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("login: status %d, body %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Token == "" {
		t.Fatalf("login response without token: %s", rec.Body.String())
	}

	if code := request(router, "GET", "/file/missing.mp4?token="+body.Token, ""); code == stdhttp.StatusUnauthorized || code == stdhttp.StatusForbidden {
		t.Fatalf("login token rejected on /file: status %d", code)
	}
}

func TestRecordingsRequireToken(t *testing.T) {
	router, key := securityRouter(t)

	if code := request(router, "GET", "/file/recording.mp4", ""); code != stdhttp.StatusUnauthorized {
		t.Fatalf("/file without token: status %d, want 401", code)
	}
	// With a valid token the handler runs and reports the missing file.
	token := signedToken(t, key, "root")
	if code := request(router, "GET", "/file/recording.mp4?token="+token, ""); code != stdhttp.StatusNotFound {
		t.Fatalf("/file with ?token=: status %d, want 404 from the handler", code)
	}
}

func TestLiveWebSocketRequiresToken(t *testing.T) {
	router, key := securityRouter(t)

	if code := request(router, "GET", "/ws", ""); code != stdhttp.StatusUnauthorized {
		t.Fatalf("/ws without token: status %d, want 401", code)
	}
	// With a valid token the request reaches the upgrader, which rejects a
	// non-WebSocket request itself.
	token := signedToken(t, key, "root")
	if code := request(router, "GET", "/ws?token="+token, ""); code == stdhttp.StatusUnauthorized || code == stdhttp.StatusForbidden {
		t.Fatalf("/ws with ?token=: status %d, want the request to pass authentication", code)
	}
}

func TestProfilerDisabledByDefault(t *testing.T) {
	t.Setenv("AGENT_ENABLE_PPROF", "")
	router, key := securityRouter(t)

	if code := request(router, "GET", "/debug/pprof/", signedToken(t, key, "root")); code != stdhttp.StatusNotFound {
		t.Fatalf("pprof without AGENT_ENABLE_PPROF: status %d, want 404", code)
	}
}

func TestProfilerRequiresTokenWhenEnabled(t *testing.T) {
	t.Setenv("AGENT_ENABLE_PPROF", "true")
	router, key := securityRouter(t)

	if code := request(router, "GET", "/debug/pprof/", ""); code != stdhttp.StatusUnauthorized {
		t.Fatalf("enabled pprof without token: status %d, want 401", code)
	}
	if code := request(router, "GET", "/debug/pprof/", signedToken(t, key, "root")); code != stdhttp.StatusOK {
		t.Fatalf("enabled pprof with token: status %d, want 200", code)
	}
}
