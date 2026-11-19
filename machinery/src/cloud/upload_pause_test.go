package cloud

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kerberos-io/agent/machinery/src/models"
)

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 11, 16, 22, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		value  string
		want   time.Duration
		wantOK bool
	}{
		{"delay seconds", "120", 120 * time.Second, true},
		{"delay seconds with spaces", "  3600 ", time.Hour, true},
		{"zero", "0", 0, true},
		{"IMF-fixdate", "Mon, 16 Nov 2026 23:00:00 GMT", time.Hour, true},
		{"RFC 850 date", "Monday, 16-Nov-26 23:30:00 GMT", 90 * time.Minute, true},
		{"asctime date", "Mon Nov 16 22:00:30 2026", 30 * time.Second, true},
		{"date in the past", "Mon, 16 Nov 2026 21:00:00 GMT", -time.Hour, true},
		{"more than the cap", "4000000", uploadPauseMaxRetryAfter, true},
		{"overflowing number", "99999999999999999999999999", uploadPauseMaxRetryAfter, true},
		{"missing", "", 0, false},
		{"negative", "-5", 0, false},
		{"fraction", "1.5", 0, false},
		{"unit suffix", "10s", 0, false},
		{"garbage", "tomorrow", 0, false},
		{"ISO date", "2026-11-17T00:00:00Z", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseRetryAfter(tt.value, now)
			if ok != tt.wantOK || got != tt.want {
				t.Fatalf("parseRetryAfter(%q) = %v, %v; want %v, %v", tt.value, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestClampRetryAfter(t *testing.T) {
	tests := []struct {
		in, want time.Duration
	}{
		{-time.Hour, uploadPauseMinBackoff},
		{0, uploadPauseMinBackoff},
		{time.Second, uploadPauseMinBackoff},
		{2 * time.Hour, 2 * time.Hour},
		{uploadPauseMaxRetryAfter, uploadPauseMaxRetryAfter},
		{40 * 24 * time.Hour, uploadPauseMaxRetryAfter},
	}
	for _, tt := range tests {
		if got := clampRetryAfter(tt.in); got != tt.want {
			t.Errorf("clampRetryAfter(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestRetryAfterFromResponseUsesDateHeader(t *testing.T) {
	// The agent clock may be wrong; an HTTP-date is measured against the
	// response Date so the pause still ends at the start of Hub's next period.
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("Date", "Mon, 16 Nov 2020 22:00:00 GMT")
	resp.Header.Set("Retry-After", "Tue, 17 Nov 2020 00:00:00 GMT")
	got, ok := retryAfterFromResponse(resp)
	if !ok || got != 2*time.Hour {
		t.Fatalf("retryAfterFromResponse = %v, %v; want 2h, true", got, ok)
	}
}

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func TestUploadPauseHonoursRetryAfter(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 11, 16, 22, 0, 0, 0, time.UTC)}
	p := newUploadPause(clock.Now)

	if got := p.Remaining(); got != 0 {
		t.Fatalf("Remaining before any 429 = %v, want 0", got)
	}
	if got := p.Throttled(2*time.Hour, true); got != 2*time.Hour {
		t.Fatalf("Throttled = %v, want 2h", got)
	}
	if got := p.Remaining(); got != 2*time.Hour {
		t.Fatalf("Remaining = %v, want 2h", got)
	}
	clock.Advance(90 * time.Minute)
	if got := p.Remaining(); got != 30*time.Minute {
		t.Fatalf("Remaining after 90m = %v, want 30m", got)
	}
	clock.Advance(30 * time.Minute)
	if got := p.Remaining(); got != 0 {
		t.Fatalf("Remaining at the deadline = %v, want 0", got)
	}

	// Absurd values are clamped.
	if got := p.Throttled(-time.Hour, true); got != uploadPauseMinBackoff {
		t.Fatalf("Throttled(past) = %v, want %v", got, uploadPauseMinBackoff)
	}
	if got := p.Throttled(365*24*time.Hour, true); got != uploadPauseMaxRetryAfter {
		t.Fatalf("Throttled(1 year) = %v, want %v", got, uploadPauseMaxRetryAfter)
	}

	p.Clear()
	if got := p.Remaining(); got != 0 {
		t.Fatalf("Remaining after Clear = %v, want 0", got)
	}
}

func TestUploadPauseBacksOffWithoutRetryAfter(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 11, 16, 22, 0, 0, 0, time.UTC)}
	p := newUploadPause(clock.Now)

	want := []time.Duration{
		5 * time.Second, 10 * time.Second, 20 * time.Second, 40 * time.Second,
		80 * time.Second, 160 * time.Second, 320 * time.Second, 640 * time.Second,
		15 * time.Minute, 15 * time.Minute,
	}
	for i, w := range want {
		if got := p.Throttled(0, false); got != w {
			t.Fatalf("attempt %d: Throttled = %v, want %v", i, got, w)
		}
		if got := p.Remaining(); got != w {
			t.Fatalf("attempt %d: Remaining = %v, want %v", i, got, w)
		}
		clock.Advance(w)
	}

	// A valid Retry-After does not disturb the back-off sequence.
	if got := p.Throttled(time.Hour, true); got != time.Hour {
		t.Fatalf("Throttled with Retry-After = %v, want 1h", got)
	}
	if got := p.Throttled(0, false); got != 15*time.Minute {
		t.Fatalf("back-off after Retry-After = %v, want 15m", got)
	}

	// Clear (Hub answered something other than 429) restarts the back-off.
	p.Clear()
	if got := p.Throttled(0, false); got != 5*time.Second {
		t.Fatalf("back-off after Clear = %v, want 5s", got)
	}
	p.Reset()
	if got := p.Remaining(); got != 0 {
		t.Fatalf("Remaining after Reset = %v, want 0", got)
	}
}

func TestUploadPauseConcurrentUse(t *testing.T) {
	p := newUploadPause(time.Now)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				switch (i + j) % 4 {
				case 0:
					p.Throttled(time.Minute, true)
				case 1:
					p.Throttled(0, false)
				case 2:
					p.Remaining()
				case 3:
					p.Clear()
				}
			}
		}(i)
	}
	wg.Wait()
}

func TestThrottledErrorOnlyFor429(t *testing.T) {
	base := errors.New("boom")
	resp := &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{}}
	if got := throttledError(resp, base); got != base {
		t.Fatalf("throttledError(403) = %v, want the original error", got)
	}
	resp = &http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{}}
	resp.Header.Set("Retry-After", "60")
	var throttled *uploadThrottledError
	if err := throttledError(resp, base); !errors.As(err, &throttled) || !errors.Is(err, base) {
		t.Fatalf("throttledError(429) = %v, want an uploadThrottledError wrapping the original", err)
	}
	if !throttled.ok || throttled.retryAfter != time.Minute {
		t.Fatalf("retryAfter = %v, %v; want 1m, true", throttled.retryAfter, throttled.ok)
	}
}

// withHubUploadPause gives a test a fresh shared pause and restores it after.
func withHubUploadPause(t *testing.T) {
	t.Helper()
	previous := hubUploadPause
	hubUploadPause = newUploadPause(time.Now)
	t.Cleanup(func() { hubUploadPause = previous })
}

func testHubConfiguration(hubURI string) *models.Configuration {
	return &models.Configuration{Config: *testHubConfig(hubURI)}
}

func TestUploadKerberosHubTusCreate429PausesWithoutRetrying(t *testing.T) {
	withHubUploadPause(t)
	previousDelay := tusBackoffBaseDelay
	tusBackoffBaseDelay = 0
	t.Cleanup(func() { tusBackoffBaseDelay = previousDelay })

	var posts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, tusUploadPath) {
			atomic.AddInt32(&posts, 1)
			w.Header().Set("Retry-After", time.Now().Add(3*time.Hour).UTC().Format(http.TimeFormat))
			http.Error(w, `{"error":"plan_limit_reached"}`, http.StatusTooManyRequests)
			return
		}
		t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	fileName := "1564859471_6-474162_oprit_577-283-727-375_1153_27.mp4"
	withRecording(t, fileName, bytes.Repeat([]byte("r"), 1024))

	uploaded, configured, err := UploadKerberosHub(testHubConfiguration(server.URL), fileName)
	if uploaded || !configured || err != nil {
		t.Fatalf("UploadKerberosHub = %v, %v, %v; want false, true, nil", uploaded, configured, err)
	}
	if got := atomic.LoadInt32(&posts); got != 1 {
		t.Fatalf("tus create requests = %d, want 1 (no in-cycle retries on 429)", got)
	}
	if remaining := hubUploadPause.Remaining(); remaining < 2*time.Hour+50*time.Minute || remaining > 3*time.Hour {
		t.Fatalf("pause = %v, want about 3h", remaining)
	}
	if _, err := os.Stat("data/recordings/" + fileName); err != nil {
		t.Fatalf("recording must stay on disk: %v", err)
	}
}

func TestUploadKerberosHubTusPatch429KeepsResumeState(t *testing.T) {
	withHubUploadPause(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Tus-Resumable", tusResumableVersion)
		switch r.Method {
		case http.MethodPost:
			w.Header().Set("Location", tusUploadPath+"upload-1")
			w.WriteHeader(http.StatusCreated)
		case http.MethodHead:
			w.Header().Set("Upload-Offset", "0")
			w.WriteHeader(http.StatusOK)
		case http.MethodPatch:
			w.WriteHeader(http.StatusTooManyRequests) // no Retry-After
		}
	}))
	defer server.Close()

	fileName := "1564859471_6-474162_oprit_577-283-727-375_1153_27.mp4"
	withRecording(t, fileName, bytes.Repeat([]byte("r"), 1024))

	uploaded, configured, err := UploadKerberosHub(testHubConfiguration(server.URL), fileName)
	if uploaded || !configured || err != nil {
		t.Fatalf("UploadKerberosHub = %v, %v, %v; want false, true, nil", uploaded, configured, err)
	}
	if remaining := hubUploadPause.Remaining(); remaining <= 0 || remaining > uploadPauseMinBackoff {
		t.Fatalf("pause without Retry-After = %v, want up to %v", remaining, uploadPauseMinBackoff)
	}
	if _, err := os.Stat(tusSidecarPath(fileName, "hub")); err != nil {
		t.Fatalf("resume state must be kept during the pause: %v", err)
	}
}

func TestUploadKerberosHubLegacyHead429Pauses(t *testing.T) {
	withHubUploadPause(t)
	t.Setenv("AGENT_DISABLE_RESUMABLE_UPLOAD", "true")

	var uploads int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/storage/upload" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			return
		}
		if r.Method == http.MethodPost {
			atomic.AddInt32(&uploads, 1)
		}
		w.Header().Set("Retry-After", "7200")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	fileName := "1564859471_6-474162_oprit_577-283-727-375_1153_27.mp4"
	withRecording(t, fileName, bytes.Repeat([]byte("r"), 1024))

	uploaded, configured, err := UploadKerberosHub(testHubConfiguration(server.URL), fileName)
	if uploaded || !configured || err != nil {
		t.Fatalf("UploadKerberosHub = %v, %v, %v; want false, true, nil", uploaded, configured, err)
	}
	if got := atomic.LoadInt32(&uploads); got != 0 {
		t.Fatalf("POST after a 429 HEAD = %d, want 0", got)
	}
	if remaining := hubUploadPause.Remaining(); remaining < 119*time.Minute || remaining > 2*time.Hour {
		t.Fatalf("pause = %v, want about 2h", remaining)
	}
}

func TestUploadKerberosHubOtherStatusesDoNotPause(t *testing.T) {
	// Older Hub versions never send 429; a refusal keeps today's behaviour
	// (recording kept, retried by the upload loop) and does not pause.
	withHubUploadPause(t)
	t.Setenv("AGENT_DISABLE_RESUMABLE_UPLOAD", "true")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	fileName := "1564859471_6-474162_oprit_577-283-727-375_1153_27.mp4"
	withRecording(t, fileName, bytes.Repeat([]byte("r"), 1024))

	hubUploadPause.Throttled(0, false) // an earlier 429 episode
	uploaded, configured, err := UploadKerberosHub(testHubConfiguration(server.URL), fileName)
	if uploaded || !configured || err != nil {
		t.Fatalf("UploadKerberosHub = %v, %v, %v; want false, true, nil", uploaded, configured, err)
	}
	if remaining := hubUploadPause.Remaining(); remaining != 0 {
		t.Fatalf("pause after a non-429 answer = %v, want 0", remaining)
	}
}
