package cloud

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kerberos-io/agent/machinery/src/models"
)

// fakeUpload tracks the state of a single resumable upload on the fake server.
type fakeUpload struct {
	size   int64
	offset int64
}

// recordedRequest captures the method and headers of a request received by the
// fake tus server, so tests can assert the client's per-method auth headers.
type recordedRequest struct {
	method string
	header http.Header
}

// fakeTus is a tiny in-memory implementation of the tus 1.0.0 server protocol,
// sufficient to exercise the agent's resumable client.
type fakeTus struct {
	mu             sync.Mutex
	uploads        map[string]*fakeUpload
	counter        int
	creates        int
	lastPatchBytes int64
	patchSizes     []int64

	// unsupported makes the creation endpoint return 404, simulating an older
	// vault without a tus endpoint.
	unsupported bool
	// failFinalize causes the next N completing PATCH requests to return 502
	// after storing the bytes, simulating a failed completion hook.
	failFinalize int

	// requests records the headers of every received request (in order) so
	// tests can assert which auth/routing headers the client sent per method.
	requests []recordedRequest
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

// patchCounts returns the number of PATCH requests received and the size of each.
func (s *fakeTus) patchCounts() (int, []int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sizes := make([]int64, len(s.patchSizes))
	copy(sizes, s.patchSizes)
	return len(s.patchSizes), sizes
}

func (s *fakeTus) createCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.creates
}

// requestsForMethod returns the recorded requests for the given HTTP method.
func (s *fakeTus) requestsForMethod(method string) []recordedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []recordedRequest
	for _, req := range s.requests {
		if req.method == method {
			out = append(out, req)
		}
	}
	return out
}

func (s *fakeTus) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, tusUploadPath)
	w.Header().Set("Tus-Resumable", tusResumableVersion)

	s.mu.Lock()
	s.requests = append(s.requests, recordedRequest{method: r.Method, header: r.Header.Clone()})
	s.mu.Unlock()

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
		s.patchSizes = append(s.patchSizes, n)
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

func TestUploadVaultResumable_Chunked(t *testing.T) {
	srv := newFakeTus()
	ts := httptest.NewServer(srv)
	defer ts.Close()

	fileName := "1564859471_6-474162_oprit_577-283-727-375_1153_27.mp4"
	// 10 KiB payload uploaded in 4 KiB chunks => 3 PATCH requests (4096+4096+2048).
	payload := bytes.Repeat([]byte("c"), 10240)
	withRecording(t, fileName, payload)
	t.Setenv("AGENT_TUS_CHUNK_SIZE_BYTES", "4096")

	uploaded, _, supported, _, err := uploadVaultResumable(testVault(ts.URL), "pk", "dev", fileName, "test", "primary")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !uploaded || !supported {
		t.Fatalf("expected chunked upload success, got uploaded=%v supported=%v", uploaded, supported)
	}
	if got := srv.totalBytes(); got != int64(len(payload)) {
		t.Fatalf("server received %d bytes, want %d", got, len(payload))
	}
	count, sizes := srv.patchCounts()
	if count != 3 {
		t.Fatalf("expected 3 chunked PATCH requests, got %d (sizes=%v)", count, sizes)
	}
	want := []int64{4096, 4096, 2048}
	for i, w := range want {
		if sizes[i] != w {
			t.Fatalf("chunk %d size = %d, want %d (sizes=%v)", i, sizes[i], w, sizes)
		}
	}
	if _, err := os.Stat(tusSidecarPath(fileName, "primary")); !os.IsNotExist(err) {
		t.Fatalf("expected sidecar removed after success, stat err = %v", err)
	}
}

func TestUploadVaultResumable_ChunkingDisabled(t *testing.T) {
	srv := newFakeTus()
	ts := httptest.NewServer(srv)
	defer ts.Close()

	fileName := "1564859471_6-474162_oprit_577-283-727-375_1153_27.mp4"
	payload := bytes.Repeat([]byte("d"), 10240)
	withRecording(t, fileName, payload)
	// 0 disables chunking: the whole file should go out in a single PATCH.
	t.Setenv("AGENT_TUS_CHUNK_SIZE_BYTES", "0")

	uploaded, _, supported, _, err := uploadVaultResumable(testVault(ts.URL), "pk", "dev", fileName, "test", "primary")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !uploaded || !supported {
		t.Fatalf("expected success, got uploaded=%v supported=%v", uploaded, supported)
	}
	count, sizes := srv.patchCounts()
	if count != 1 {
		t.Fatalf("expected a single PATCH when chunking is disabled, got %d (sizes=%v)", count, sizes)
	}
	if sizes[0] != int64(len(payload)) {
		t.Fatalf("single PATCH size = %d, want %d", sizes[0], len(payload))
	}
}

func TestTusChunkSize(t *testing.T) {
	cases := []struct {
		name string
		env  string
		set  bool
		want int64
	}{
		{name: "default when unset", set: false, want: tusDefaultChunkSize},
		{name: "default on invalid", env: "notanumber", set: true, want: tusDefaultChunkSize},
		{name: "explicit value", env: "65536", set: true, want: 65536},
		{name: "zero disables", env: "0", set: true, want: 0},
		{name: "negative disables", env: "-5", set: true, want: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set {
				t.Setenv("AGENT_TUS_CHUNK_SIZE_BYTES", tc.env)
			} else {
				t.Setenv("AGENT_TUS_CHUNK_SIZE_BYTES", "")
			}
			if got := tusChunkSize(); got != tc.want {
				t.Fatalf("tusChunkSize() = %d, want %d", got, tc.want)
			}
		})
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

// TestUploadVaultResumable_NetworkErrorKeepsRetryBudget verifies that when the
// vault is unreachable (mimicking the internet being disconnected) the resumable
// upload reports responded=false. That is what stops the caller
// (UploadKerberosVault) from consuming its retry budget and entering the long
// back-off timeout on a transient network outage, so the recording keeps being
// retried until connectivity returns.
func TestUploadVaultResumable_NetworkErrorKeepsRetryBudget(t *testing.T) {
	// Bind then immediately release a loopback port so every connection to it is
	// refused, producing a transport-level error (no HTTP response).
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	if cerr := ln.Close(); cerr != nil {
		t.Fatalf("close listener: %v", cerr)
	}

	// Keep the between-attempt back-off tiny so the test stays fast.
	oldDelay := tusBackoffBaseDelay
	tusBackoffBaseDelay = time.Millisecond
	defer func() { tusBackoffBaseDelay = oldDelay }()

	fileName := "1564859471_6-474162_oprit_577-283-727-375_1153_27.mp4"
	withRecording(t, fileName, bytes.Repeat([]byte("n"), 2048))

	uploaded, responded, supported, _, err := uploadVaultResumable(testVault("http://"+addr), "pk", "dev", fileName, "test", "primary")
	if uploaded {
		t.Fatal("expected uploaded=false when the vault is unreachable")
	}
	if !supported {
		t.Fatal("a transport error is not a missing tus endpoint; expected supported=true")
	}
	if responded {
		t.Fatal("expected responded=false for a pure network error so the retry budget is preserved")
	}
	if err == nil {
		t.Fatal("expected an error when the vault is unreachable")
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

func testHubConfig(hubURI string) *models.Config {
	return &models.Config{
		Key:           "device-key",
		HubURI:        hubURI,
		HubKey:        "hubpub",
		HubPrivateKey: "hubpriv",
		S3:            &models.S3{Region: "eu-west"},
	}
}

// decodeTusMetadata parses a tus Upload-Metadata header value ("key b64,key b64")
// back into a map of decoded key/value pairs.
func decodeTusMetadata(meta string) map[string]string {
	out := map[string]string{}
	if meta == "" {
		return out
	}
	for _, pair := range strings.Split(meta, ",") {
		parts := strings.SplitN(strings.TrimSpace(pair), " ", 2)
		if parts[0] == "" {
			continue
		}
		val := ""
		if len(parts) == 2 {
			if b, err := base64.StdEncoding.DecodeString(parts[1]); err == nil {
				val = string(b)
			}
		}
		out[parts[0]] = val
	}
	return out
}

func TestUploadHubResumable_HappyPath(t *testing.T) {
	srv := newFakeTus()
	ts := httptest.NewServer(srv)
	defer ts.Close()

	fileName := "1564859471_6-474162_oprit_577-283-727-375_1153_27.mp4"
	payload := bytes.Repeat([]byte("h"), 4096)
	withRecording(t, fileName, payload)

	uploaded, _, supported, _, err := uploadHubResumable(testHubConfig(ts.URL), fileName, "test", "hub")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !uploaded || !supported {
		t.Fatalf("uploaded/supported = %v/%v, want both true", uploaded, supported)
	}
	if got := srv.totalBytes(); got != int64(len(payload)) {
		t.Fatalf("server received %d bytes, want %d", got, len(payload))
	}

	// The Hub auth headers must be present on every request type (POST/HEAD/PATCH),
	// because Kerberos Hub validates them on each proxied request. Conversely the
	// vault credentials/routing are injected by Kerberos Hub on the agent's behalf
	// and must never be sent by the agent on the hub path.
	for _, method := range []string{http.MethodPost, http.MethodHead, http.MethodPatch} {
		reqs := srv.requestsForMethod(method)
		if len(reqs) == 0 {
			t.Fatalf("expected at least one %s request", method)
		}
		for _, req := range reqs {
			if got := req.header.Get("X-Kerberos-Hub-PublicKey"); got != "hubpub" {
				t.Errorf("%s: X-Kerberos-Hub-PublicKey = %q, want %q", method, got, "hubpub")
			}
			if got := req.header.Get("X-Kerberos-Hub-PrivateKey"); got != "hubpriv" {
				t.Errorf("%s: X-Kerberos-Hub-PrivateKey = %q, want %q", method, got, "hubpriv")
			}
			if got := req.header.Get("X-Kerberos-Hub-Region"); got != "eu-west" {
				t.Errorf("%s: X-Kerberos-Hub-Region = %q, want %q", method, got, "eu-west")
			}
			if got := req.header.Get("X-Kerberos-Storage-Device"); got != "device-key" {
				t.Errorf("%s: X-Kerberos-Storage-Device = %q, want %q", method, got, "device-key")
			}
			for _, h := range []string{
				"X-Kerberos-Storage-AccessKey",
				"X-Kerberos-Storage-SecretAccessKey",
				"X-Kerberos-Storage-CloudKey",
				"X-Kerberos-Storage-Provider",
				"X-Kerberos-Storage-Directory",
			} {
				if got := req.header.Get(h); got != "" {
					t.Errorf("%s: %s should be empty on the hub path, got %q", method, h, got)
				}
			}
		}
	}

	// The creation request carries the upload metadata; on the hub path it must
	// omit directory/provider/cloudkey (Hub resolves those) but include
	// filename/device/capture. The filename header is also set on create.
	posts := srv.requestsForMethod(http.MethodPost)
	if got := posts[0].header.Get("X-Kerberos-Storage-FileName"); got != fileName {
		t.Errorf("POST X-Kerberos-Storage-FileName = %q, want %q", got, fileName)
	}
	meta := decodeTusMetadata(posts[0].header.Get("Upload-Metadata"))
	for _, omitted := range []string{"directory", "provider", "cloudkey"} {
		if _, ok := meta[omitted]; ok {
			t.Errorf("hub metadata must omit %q, got %v", omitted, meta)
		}
	}
	if meta["filename"] != fileName {
		t.Errorf("hub metadata filename = %q, want %q", meta["filename"], fileName)
	}
	if meta["device"] != "device-key" {
		t.Errorf("hub metadata device = %q, want %q", meta["device"], "device-key")
	}
	if meta["capture"] != "IPCamera" {
		t.Errorf("hub metadata capture = %q, want %q", meta["capture"], "IPCamera")
	}
}

func TestUploadHubResumable_Unsupported(t *testing.T) {
	srv := newFakeTus()
	srv.unsupported = true
	ts := httptest.NewServer(srv)
	defer ts.Close()

	fileName := "f.mp4"
	withRecording(t, fileName, []byte("hello"))

	uploaded, _, supported, _, _ := uploadHubResumable(testHubConfig(ts.URL), fileName, "test", "hub")
	if uploaded {
		t.Fatal("expected uploaded=false against a hub without a tus endpoint")
	}
	if supported {
		t.Fatal("expected supported=false so the caller falls back to the legacy upload")
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
