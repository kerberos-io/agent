package cloud

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/kerberos-io/agent/machinery/src/models"
)

// fakeUpload tracks the state of a single resumable upload on the fake server.
type fakeUpload struct {
	size   int64
	offset int64
}

// fakeTus is a tiny in-memory implementation of the tus 1.0.0 server protocol,
// sufficient to exercise the agent's resumable client.
type fakeTus struct {
	mu             sync.Mutex
	uploads        map[string]*fakeUpload
	counter        int
	creates        int
	lastPatchBytes int64

	// unsupported makes the creation endpoint return 404, simulating an older
	// vault without a tus endpoint.
	unsupported bool
	// failFinalize causes the next N completing PATCH requests to return 502
	// after storing the bytes, simulating a failed completion hook.
	failFinalize int
}

func newFakeTus() *fakeTus {
	return &fakeTus{uploads: map[string]*fakeUpload{}}
}

func (s *fakeTus) seed(size, offset int64) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counter++
	id := fmt.Sprintf("seed-%d", s.counter)
	s.uploads[id] = &fakeUpload{size: size, offset: offset}
	return id
}

func (s *fakeTus) totalBytes() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	var total int64
	for _, u := range s.uploads {
		total += u.offset
	}
	return total
}

func (s *fakeTus) lastPatch() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastPatchBytes
}

func (s *fakeTus) createCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.creates
}

func (s *fakeTus) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, tusUploadPath)
	w.Header().Set("Tus-Resumable", tusResumableVersion)

	switch r.Method {
	case http.MethodPost:
		if s.unsupported {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		length, _ := strconv.ParseInt(r.Header.Get("Upload-Length"), 10, 64)
		s.mu.Lock()
		s.counter++
		s.creates++
		newID := fmt.Sprintf("up-%d", s.counter)
		s.uploads[newID] = &fakeUpload{size: length}
		s.mu.Unlock()
		w.Header().Set("Location", tusUploadPath+newID)
		w.WriteHeader(http.StatusCreated)

	case http.MethodHead:
		s.mu.Lock()
		u, ok := s.uploads[id]
		s.mu.Unlock()
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Upload-Offset", strconv.FormatInt(u.offset, 10))
		w.Header().Set("Upload-Length", strconv.FormatInt(u.size, 10))
		w.WriteHeader(http.StatusOK)

	case http.MethodPatch:
		s.mu.Lock()
		u, ok := s.uploads[id]
		s.mu.Unlock()
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		n, _ := io.Copy(io.Discard, r.Body)
		s.mu.Lock()
		u.offset += n
		s.lastPatchBytes = n
		complete := u.offset >= u.size
		failNow := complete && s.failFinalize > 0
		if failNow {
			s.failFinalize--
		}
		offset := u.offset
		s.mu.Unlock()

		w.Header().Set("Upload-Offset", strconv.FormatInt(offset, 10))
		if failNow {
			// Bytes are stored but the (simulated) completion hook failed.
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	case http.MethodDelete:
		s.mu.Lock()
		delete(s.uploads, id)
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// withRecording switches into a fresh temp working directory containing a
// recording at data/recordings/<fileName>. The working directory is restored on
// cleanup. Tests using this helper must not run in parallel.
func withRecording(t *testing.T, fileName string, payload []byte) {
	t.Helper()
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	if err := os.MkdirAll("data/recordings", 0o755); err != nil {
		t.Fatalf("mkdir recordings: %v", err)
	}
	if err := os.WriteFile(filepath.Join("data/recordings", fileName), payload, 0o644); err != nil {
		t.Fatalf("write recording: %v", err)
	}
}

func testVault(uri string) models.KStorage {
	return models.KStorage{
		URI:             uri,
		AccessKey:       "ak",
		SecretAccessKey: "sk",
		Provider:        "gcp",
		Directory:       "dir",
	}
}

func TestUploadVaultResumable_HappyPath(t *testing.T) {
	srv := newFakeTus()
	ts := httptest.NewServer(srv)
	defer ts.Close()

	fileName := "1564859471_6-474162_oprit_577-283-727-375_1153_27.mp4"
	payload := bytes.Repeat([]byte("x"), 4096)
	withRecording(t, fileName, payload)

	uploaded, responded, supported, _, err := uploadVaultResumable(testVault(ts.URL), "pk", "dev", fileName, "test", "primary")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !uploaded || !responded || !supported {
		t.Fatalf("uploaded/responded/supported = %v/%v/%v, want all true", uploaded, responded, supported)
	}
	if got := srv.totalBytes(); got != int64(len(payload)) {
		t.Fatalf("server received %d bytes, want %d", got, len(payload))
	}
	if _, err := os.Stat(tusSidecarPath(fileName, "primary")); !os.IsNotExist(err) {
		t.Fatalf("expected sidecar to be removed after success, stat err = %v", err)
	}
}

func TestUploadVaultResumable_Unsupported(t *testing.T) {
	srv := newFakeTus()
	srv.unsupported = true
	ts := httptest.NewServer(srv)
	defer ts.Close()

	fileName := "f.mp4"
	withRecording(t, fileName, []byte("hello"))

	uploaded, _, supported, _, _ := uploadVaultResumable(testVault(ts.URL), "pk", "dev", fileName, "test", "primary")
	if uploaded {
		t.Fatal("expected uploaded=false against a vault without a tus endpoint")
	}
	if supported {
		t.Fatal("expected supported=false so the caller falls back to the legacy upload")
	}
}

func TestUploadVaultResumable_FinalizeRetry(t *testing.T) {
	srv := newFakeTus()
	srv.failFinalize = 1
	ts := httptest.NewServer(srv)
	defer ts.Close()

	fileName := "1564859471_6-474162_oprit_577-283-727-375_1153_27.mp4"
	payload := bytes.Repeat([]byte("y"), 2048)
	withRecording(t, fileName, payload)

	uploaded, _, supported, _, err := uploadVaultResumable(testVault(ts.URL), "pk", "dev", fileName, "test", "primary")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !uploaded || !supported {
		t.Fatalf("expected success after a failed finalize + restart, got uploaded=%v supported=%v", uploaded, supported)
	}
	if got := srv.createCount(); got < 2 {
		t.Fatalf("expected at least 2 create requests (restart after failed finalize), got %d", got)
	}
}

func TestUploadVaultResumable_ResumeFromSidecar(t *testing.T) {
	srv := newFakeTus()
	ts := httptest.NewServer(srv)
	defer ts.Close()

	fileName := "1564859471_6-474162_oprit_577-283-727-375_1153_27.mp4"
	total := 8192
	half := 4096
	payload := bytes.Repeat([]byte("z"), total)
	withRecording(t, fileName, payload)

	// Simulate a previous run that uploaded half the file before being interrupted.
	id := srv.seed(int64(total), int64(half))
	baseURL := strings.TrimRight(ts.URL, "/") + tusUploadPath
	saveTusResumeState(tusSidecarPath(fileName, "primary"), tusResumeState{
		UploadURL: strings.TrimRight(baseURL, "/") + "/" + id,
		VaultURI:  baseURL,
		Size:      int64(total),
	})

	uploaded, _, supported, _, err := uploadVaultResumable(testVault(ts.URL), "pk", "dev", fileName, "test", "primary")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !uploaded || !supported {
		t.Fatalf("expected resume success, got uploaded=%v supported=%v", uploaded, supported)
	}
	if got := srv.lastPatch(); got != int64(total-half) {
		t.Fatalf("resume should only send the remaining %d bytes, sent %d", total-half, got)
	}
	if srv.createCount() != 0 {
		t.Fatalf("resume should not create a new upload, got %d creates", srv.createCount())
	}
}

func TestEncodeTusMetadata(t *testing.T) {
	got := encodeTusMetadata(map[string]string{
		"b":     "2",
		"a":     "1",
		"empty": "",
	})
	// keys sorted, empty values skipped, values base64-encoded.
	want := "a MQ==,b Mg=="
	if got != want {
		t.Fatalf("encodeTusMetadata = %q, want %q", got, want)
	}
}

func TestResolveTusLocation(t *testing.T) {
	cases := []struct {
		name     string
		base     string
		location string
		want     string
	}{
		{
			name:     "absolute path location",
			base:     "http://host/storage/tus/",
			location: "/storage/tus/abc",
			want:     "http://host/storage/tus/abc",
		},
		{
			name:     "absolute url keeps configured host",
			base:     "http://host/storage/tus/",
			location: "http://internal:8080/storage/tus/xyz",
			want:     "http://host/storage/tus/xyz",
		},
		{
			name:     "relative id",
			base:     "http://host/api/storage/tus/",
			location: "abc",
			want:     "http://host/api/storage/tus/abc",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveTusLocation(tc.base, tc.location); got != tc.want {
				t.Fatalf("resolveTusLocation(%q, %q) = %q, want %q", tc.base, tc.location, got, tc.want)
			}
		})
	}
}

func TestTusResumeStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(old)

	path := tusSidecarPath("file.mp4", "primary")
	state := tusResumeState{UploadURL: "http://host/storage/tus/abc", VaultURI: "http://host/storage/tus/", Size: 123}
	saveTusResumeState(path, state)

	if got := loadTusResumeState(path, state.VaultURI); got != state.UploadURL {
		t.Fatalf("loadTusResumeState = %q, want %q", got, state.UploadURL)
	}
	// A mismatched vault URI must not be reused.
	if got := loadTusResumeState(path, "http://other/storage/tus/"); got != "" {
		t.Fatalf("loadTusResumeState with mismatched vault = %q, want empty", got)
	}
}
