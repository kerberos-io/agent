package cloud

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// Kerberos Hub answers a recording upload with 429 Too Many Requests when the
// organisation is over its daily or monthly upload limit, and sets Retry-After
// to the start of the next period. The agent then pauses uploads to Hub until
// that moment instead of retrying every few seconds. Recordings stay queued on
// disk and are uploaded, in the usual order, once the pause is over.
const (
	// uploadPauseMinBackoff is the first back-off when a 429 carries no usable
	// Retry-After. It equals the existing retry interval after a refused upload.
	uploadPauseMinBackoff = 5 * time.Second
	// uploadPauseMaxBackoff caps the exponential back-off without Retry-After.
	uploadPauseMaxBackoff = 15 * time.Minute
	// uploadPauseMaxRetryAfter caps an explicit Retry-After. A monthly limit
	// never needs more than 31 days; anything longer is treated as bogus.
	uploadPauseMaxRetryAfter = 32 * 24 * time.Hour
)

// hubUploadPause is shared by every uploader that talks to Kerberos Hub.
var hubUploadPause = newUploadPause(time.Now)

// uploadPause holds the deadline until which uploads to one target are paused.
// It is safe for concurrent use.
type uploadPause struct {
	mu      sync.Mutex
	now     func() time.Time
	until   time.Time
	backoff time.Duration // next back-off when Retry-After is missing
	active  bool          // a 429 episode is in progress (for start/end logs)
}

func newUploadPause(now func() time.Time) *uploadPause {
	return &uploadPause{now: now}
}

// Remaining returns how long uploads stay paused; zero when not paused.
func (p *uploadPause) Remaining() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.active {
		return 0
	}
	if remaining := p.until.Sub(p.now()); remaining > 0 {
		return remaining
	}
	return 0
}

// Throttled records a 429 answer. retryAfter is the delay parsed from the
// Retry-After header; ok is false when the header was missing or invalid, in
// which case an exponential back-off is used. It returns the pause duration.
func (p *uploadPause) Throttled(retryAfter time.Duration, ok bool) time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()

	var wait time.Duration
	if ok {
		wait = clampRetryAfter(retryAfter)
	} else {
		if p.backoff < uploadPauseMinBackoff {
			p.backoff = uploadPauseMinBackoff
		}
		wait = p.backoff
		p.backoff *= 2
		if p.backoff > uploadPauseMaxBackoff {
			p.backoff = uploadPauseMaxBackoff
		}
	}

	now := p.now()
	p.until = now.Add(wait)
	fields := log.Fields{
		"component":           "upload",
		"target":              "kerberos_hub",
		"status_code":         http.StatusTooManyRequests,
		"paused_until":        now.Add(wait).Format(time.RFC3339),
		"pause":               wait.Round(time.Second).String(),
		"retry_after_present": ok,
	}
	if !p.active {
		p.active = true
		log.WithFields(fields).WithField("event", "upload_paused").
			Warn("Kerberos Hub refused the upload (upload limit reached); pausing uploads, recordings stay queued on disk")
	} else {
		log.WithFields(fields).WithField("event", "upload_pause_extended").
			Debug("Kerberos Hub still refuses uploads; pause extended")
	}
	return wait
}

// Clear ends a pause after Hub answered an upload with anything but 429.
func (p *uploadPause) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.active {
		log.WithFields(log.Fields{
			"component": "upload",
			"target":    "kerberos_hub",
			"event":     "upload_resumed",
		}).Info("Kerberos Hub accepts uploads again; upload pause ended")
	}
	p.active = false
	p.until = time.Time{}
	p.backoff = 0
}

// Reset drops any pause without logging, for example when the upload loop is
// restarted after a reconfiguration.
func (p *uploadPause) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.active = false
	p.until = time.Time{}
	p.backoff = 0
}

func clampRetryAfter(d time.Duration) time.Duration {
	if d < uploadPauseMinBackoff {
		// Zero or a date in the past: wait as long as a normal refused upload.
		return uploadPauseMinBackoff
	}
	if d > uploadPauseMaxRetryAfter {
		return uploadPauseMaxRetryAfter
	}
	return d
}

// parseRetryAfter parses a Retry-After value (RFC 9110, section 10.2.3):
// either delay-seconds or an HTTP-date. now is the reference time for an
// HTTP-date; callers pass the response Date header when present, so a wrong
// clock on the agent does not shift the deadline. A negative number or any
// other value is invalid. The result can be negative for a date in the past;
// Throttled clamps it.
func parseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if allDigits(value) {
		seconds, err := strconv.ParseInt(value, 10, 64)
		if err != nil || seconds > int64(uploadPauseMaxRetryAfter/time.Second) {
			// Too large to be meaningful (or to fit a Duration): cap it.
			return uploadPauseMaxRetryAfter, true
		}
		return time.Duration(seconds) * time.Second, true
	}
	at, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}
	return at.Sub(now), true
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return s != ""
}

// retryAfterFromResponse reads Retry-After from a response, using the
// response Date header (when valid) as the reference for an HTTP-date.
func retryAfterFromResponse(resp *http.Response) (time.Duration, bool) {
	now := time.Now()
	if date, err := http.ParseTime(resp.Header.Get("Date")); err == nil {
		now = date
	}
	return parseRetryAfter(resp.Header.Get("Retry-After"), now)
}

// uploadThrottledError marks an upload refused with 429 Too Many Requests and
// carries the parsed Retry-After.
type uploadThrottledError struct {
	retryAfter time.Duration
	ok         bool
	err        error
}

func (e *uploadThrottledError) Error() string {
	if e.err != nil {
		return e.err.Error()
	}
	return fmt.Sprintf("upload refused: %d %s", http.StatusTooManyRequests, http.StatusText(http.StatusTooManyRequests))
}

func (e *uploadThrottledError) Unwrap() error { return e.err }

// throttledError wraps err in an uploadThrottledError when resp is a 429;
// otherwise it returns err unchanged.
func throttledError(resp *http.Response, err error) error {
	if resp == nil || resp.StatusCode != http.StatusTooManyRequests {
		return err
	}
	retryAfter, ok := retryAfterFromResponse(resp)
	return &uploadThrottledError{retryAfter: retryAfter, ok: ok, err: err}
}
