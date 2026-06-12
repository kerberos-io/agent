package cloud

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
)

// tusResumableVersion is the tus protocol version implemented by this client.
const tusResumableVersion = "1.0.0"

// tusUploadPath is appended to the configured Kerberos Vault URI to reach the
// resumable upload endpoint. It mirrors how the legacy uploader appends
// "/storage".
const tusUploadPath = "/storage/tus/"

// tusResumeState is persisted in a sidecar file next to the agent data so an
// interrupted upload can be resumed across retries and even agent restarts.
type tusResumeState struct {
	UploadURL string `json:"upload_url"`
	VaultURI  string `json:"vault_uri"`
	Size      int64  `json:"size"`
}

// resumableUploadsEnabled reports whether the resumable (tus) upload path should
// be attempted. It is enabled by default and can be disabled (falling back to
// the legacy single POST) by setting AGENT_DISABLE_RESUMABLE_UPLOAD=true.
func resumableUploadsEnabled() bool {
	return os.Getenv("AGENT_DISABLE_RESUMABLE_UPLOAD") != "true"
}

// tusDefaultChunkSize is the number of bytes uploaded per PATCH request when no
// explicit size is configured. Splitting the upload into chunks keeps each HTTP
// request small enough for intermediary proxies/load balancers and checkpoints
// progress frequently, so an interruption resumes with minimal re-upload.
const tusDefaultChunkSize int64 = 1 << 20 // 1 MiB

const tusProgressBucketPercent int64 = 10

// tusChunkSize returns the number of bytes to send per PATCH request. It
// defaults to tusDefaultChunkSize (1 MiB) and can be overridden with the
// AGENT_TUS_CHUNK_SIZE_BYTES environment variable. A value of 0 (or negative)
// disables chunking and sends the remaining bytes in a single PATCH.
func tusChunkSize() int64 {
	v := os.Getenv("AGENT_TUS_CHUNK_SIZE_BYTES")
	if v == "" {
		return tusDefaultChunkSize
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return tusDefaultChunkSize
	}
	if n <= 0 {
		return 0 // chunking disabled: send everything in one PATCH
	}
	return n
}

func tusProgressBucket(offset, size int64) int64 {
	if size <= 0 {
		return 100
	}
	percent := (offset * 100) / size
	if percent > 100 {
		percent = 100
	}
	return percent / tusProgressBucketPercent
}

func logTusUploadProgress(label string, offset, size int64, loggedBucket *int64) {
	bucket := tusProgressBucket(offset, size)
	if bucket <= *loggedBucket {
		return
	}
	*loggedBucket = bucket
	percent := bucket * tusProgressBucketPercent
	if percent > 100 {
		percent = 100
	}
	log.Log.Infof("%s: resumable upload progress %d%% (%d/%d bytes)", label, percent, offset, size)
}

// uploadVaultResumable uploads a recording to a Kerberos Vault using the tus
// resumable upload protocol.
//
// Return values:
//   - uploaded:  the recording was fully received and persisted by the vault.
//   - responded: the vault returned a definitive HTTP response (used by the
//     caller to advance its retry/secondary-failover policy).
//   - supported: the vault exposes a tus endpoint. When false, the caller should
//     fall back to the legacy single-POST upload (older vault deployments).
//   - body:      a short message for logging.
func uploadVaultResumable(vault models.KStorage, publicKey, deviceKey, fileName, label, slot string) (uploaded bool, responded bool, supported bool, body string, err error) {
	fullname := "data/recordings/" + fileName

	file, ferr := os.Open(fullname)
	if file != nil {
		defer file.Close()
	}
	if ferr != nil {
		msg := label + ": resumable upload failed, file doesn't exist anymore"
		log.Log.Info(msg)
		// The file is gone, so the legacy path cannot help either. Report it as
		// "supported" to avoid a pointless fallback attempt.
		return false, false, true, "", errors.New(msg)
	}

	info, serr := file.Stat()
	if serr != nil {
		return false, false, true, "", serr
	}
	size := info.Size()

	baseURL := strings.TrimRight(vault.URI, "/") + tusUploadPath
	client := newVaultHTTPClient(0)

	metadata := encodeTusMetadata(map[string]string{
		"filename":  fileName,
		"device":    deviceKey,
		"directory": vault.Directory,
		"provider":  vault.Provider,
		"capture":   "IPCamera",
		"cloudkey":  publicKey,
	})

	sidecar := tusSidecarPath(fileName, slot)
	uploadURL := loadTusResumeState(sidecar, baseURL)

	const maxAttempts = 4
	restartedAfterComplete := false

	for attempt := 0; attempt < maxAttempts; attempt++ {
		// (1) Ensure we have an active upload URL, creating one if needed.
		if uploadURL == "" {
			created, status, cerr := tusCreate(client, baseURL, size, metadata, vault, publicKey, deviceKey, fileName)
			if cerr != nil {
				if status == http.StatusNotFound || status == http.StatusMethodNotAllowed || status == http.StatusNotImplemented {
					// The vault does not implement tus; let the caller fall back.
					return false, false, false, "", cerr
				}
				log.Log.Info(label + ": resumable create failed, " + cerr.Error())
				tusBackoff(attempt)
				continue
			}
			uploadURL = created
			saveTusResumeState(sidecar, tusResumeState{UploadURL: uploadURL, VaultURI: baseURL, Size: size})
		}

		// (2) Query the current server-side offset.
		offset, status, herr := tusHead(client, uploadURL, vault, publicKey, deviceKey)
		if herr != nil {
			if status == http.StatusNotFound || status == http.StatusGone {
				// The upload expired/was removed server-side; start over.
				removeTusResumeState(sidecar)
				uploadURL = ""
				continue
			}
			log.Log.Info(label + ": resumable head failed, " + herr.Error())
			tusBackoff(attempt)
			continue
		}

		// (3) All bytes are present but the upload was not finalized (e.g. the
		// completion hook failed). A completed tus upload cannot be re-finalized
		// with another PATCH, so delete it and re-upload to force a clean finalize.
		if offset >= size {
			if restartedAfterComplete {
				return false, true, true, "resumable finalize did not complete", errors.New(label + ": resumable finalize did not complete")
			}
			tusTerminate(client, uploadURL, vault, publicKey, deviceKey)
			removeTusResumeState(sidecar)
			uploadURL = ""
			restartedAfterComplete = true
			continue
		}

		// (4) Stream the remaining bytes to the vault via PATCH, reading directly
		// from disk so the recording is never fully buffered in memory. When a chunk
		// size is configured the data is sent across several PATCH requests,
		// checkpointing the offset after each one so an interruption resumes from the
		// last completed chunk instead of re-uploading everything.
		chunkSize := tusChunkSize()
		progressed := false
		patchFailed := false
		var lastBody string
		loggedProgressBucket := tusProgressBucket(offset, size)
		for offset < size {
			// Re-seek every chunk so the on-disk position always matches the
			// server-acknowledged offset, even if a PATCH was partially accepted.
			if _, sErr := file.Seek(offset, io.SeekStart); sErr != nil {
				return false, false, true, "", sErr
			}
			patchLen := size - offset
			if chunkSize > 0 && chunkSize < patchLen {
				patchLen = chunkSize
			}
			newOffset, status, respBody, perr := tusPatch(client, uploadURL, offset, patchLen, file, vault, publicKey, deviceKey)
			if perr != nil {
				if status >= 400 {
					// Definitive rejection (e.g. provider push failed during finalize).
					// Re-evaluate via HEAD on the next iteration to decide retry/restart.
					log.Log.Info(label + ": resumable patch rejected, " + perr.Error())
				} else {
					log.Log.Info(label + ": resumable patch failed, " + perr.Error())
				}
				tusBackoff(attempt)
				patchFailed = true
				break
			}
			if newOffset > offset {
				progressed = true
			}
			offset = newOffset
			lastBody = respBody
			logTusUploadProgress(label, offset, size, &loggedProgressBucket)
			if offset < size {
				// Partial progress: persist so a later retry resumes from here.
				saveTusResumeState(sidecar, tusResumeState{UploadURL: uploadURL, VaultURI: baseURL, Size: size})
			}
		}
		if patchFailed {
			if progressed {
				// Forward progress refreshes the retry budget: maxAttempts bounds the
				// number of consecutive failures, not the number of chunks needed for
				// a large recording.
				attempt = -1
			}
			continue
		}

		// All declared bytes have been sent and acknowledged: the upload is done.
		removeTusResumeState(sidecar)
		return true, true, true, lastBody, nil
	}

	return false, true, true, "resumable upload did not complete after retries", errors.New(label + ": resumable upload did not complete after retries")
}

// tusCreate performs the tus "creation" request (POST). On success it returns
// the resolved upload URL the agent should use for subsequent HEAD/PATCH calls.
func tusCreate(client *http.Client, baseURL string, size int64, metadata string, vault models.KStorage, publicKey, deviceKey, fileName string) (string, int, error) {
	req, err := http.NewRequest("POST", baseURL, nil)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Tus-Resumable", tusResumableVersion)
	req.Header.Set("Upload-Length", strconv.FormatInt(size, 10))
	if metadata != "" {
		req.Header.Set("Upload-Metadata", metadata)
	}
	setVaultTusHeaders(req.Header, vault, publicKey, deviceKey, fileName)

	resp, err := client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return "", 0, err
	}
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusCreated {
		return "", resp.StatusCode, fmt.Errorf("unexpected status creating upload: %s", resp.Status)
	}
	location := resp.Header.Get("Location")
	if location == "" {
		return "", resp.StatusCode, errors.New("missing Location header in create response")
	}
	return resolveTusLocation(baseURL, location), resp.StatusCode, nil
}

// tusHead performs the tus "offset" request (HEAD) and returns the current
// server-side upload offset.
func tusHead(client *http.Client, uploadURL string, vault models.KStorage, publicKey, deviceKey string) (int64, int, error) {
	req, err := http.NewRequest("HEAD", uploadURL, nil)
	if err != nil {
		return 0, 0, err
	}
	req.Header.Set("Tus-Resumable", tusResumableVersion)
	setVaultTusHeaders(req.Header, vault, publicKey, deviceKey, "")

	resp, err := client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return 0, 0, err
	}
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return 0, resp.StatusCode, fmt.Errorf("unexpected status on HEAD: %s", resp.Status)
	}
	offsetStr := resp.Header.Get("Upload-Offset")
	offset, perr := strconv.ParseInt(offsetStr, 10, 64)
	if perr != nil {
		return 0, resp.StatusCode, fmt.Errorf("invalid Upload-Offset header: %q", offsetStr)
	}
	return offset, resp.StatusCode, nil
}

// tusPatch streams up to length bytes of the file (starting at offset) to the
// upload URL using a single PATCH request. The body is read straight from the
// *os.File, so the recording is never fully buffered in memory.
func tusPatch(client *http.Client, uploadURL string, offset, length int64, file io.Reader, vault models.KStorage, publicKey, deviceKey string) (int64, int, string, error) {
	req, err := http.NewRequest("PATCH", uploadURL, io.LimitReader(file, length))
	if err != nil {
		return offset, 0, "", err
	}
	req.ContentLength = length
	req.Header.Set("Tus-Resumable", tusResumableVersion)
	req.Header.Set("Content-Type", "application/offset+octet-stream")
	req.Header.Set("Upload-Offset", strconv.FormatInt(offset, 10))
	setVaultTusHeaders(req.Header, vault, publicKey, deviceKey, "")

	resp, err := client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return offset, 0, "", err
	}
	bodyBytes, _ := io.ReadAll(resp.Body)
	respBody := string(bodyBytes)

	if resp.StatusCode != http.StatusNoContent {
		return offset, resp.StatusCode, respBody, fmt.Errorf("unexpected status on PATCH: %s, %s", resp.Status, respBody)
	}
	newOffsetStr := resp.Header.Get("Upload-Offset")
	newOffset, perr := strconv.ParseInt(newOffsetStr, 10, 64)
	if perr != nil {
		// A 204 without a parseable offset means this PATCH was fully accepted.
		return offset + length, resp.StatusCode, respBody, nil
	}
	return newOffset, resp.StatusCode, respBody, nil
}

// tusTerminate best-effort deletes an upload server-side (DELETE).
func tusTerminate(client *http.Client, uploadURL string, vault models.KStorage, publicKey, deviceKey string) {
	req, err := http.NewRequest("DELETE", uploadURL, nil)
	if err != nil {
		return
	}
	req.Header.Set("Tus-Resumable", tusResumableVersion)
	setVaultTusHeaders(req.Header, vault, publicKey, deviceKey, "")

	resp, derr := client.Do(req)
	if resp != nil {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	_ = derr
}

// setVaultTusHeaders sets the Kerberos Vault authentication and routing headers
// on every tus request. Credentials are sent on each request (and never stored
// server-side in the upload metadata). When fileName is empty it is omitted, as
// it is only useful on the creation request (routing also travels in the tus
// Upload-Metadata).
func setVaultTusHeaders(h http.Header, vault models.KStorage, publicKey, deviceKey, fileName string) {
	h.Set("X-Kerberos-Storage-CloudKey", publicKey)
	h.Set("X-Kerberos-Storage-AccessKey", vault.AccessKey)
	h.Set("X-Kerberos-Storage-SecretAccessKey", vault.SecretAccessKey)
	h.Set("X-Kerberos-Storage-Provider", vault.Provider)
	h.Set("X-Kerberos-Storage-Device", deviceKey)
	h.Set("X-Kerberos-Storage-Directory", vault.Directory)
	h.Set("X-Kerberos-Storage-Capture", "IPCamera")
	if fileName != "" {
		h.Set("X-Kerberos-Storage-FileName", fileName)
	}
}

// encodeTusMetadata serializes a map into the tus Upload-Metadata header format:
// a comma separated list of "key base64(value)" pairs. Keys are sorted for a
// deterministic header value. Empty values are skipped.
func encodeTusMetadata(pairs map[string]string) string {
	parts := make([]string, 0, len(pairs))
	for k, v := range pairs {
		if v == "" {
			continue
		}
		parts = append(parts, k+" "+base64.StdEncoding.EncodeToString([]byte(v)))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

// resolveTusLocation turns the Location header returned by the create request
// into an absolute URL. To keep talking to the agent's configured vault host
// (and avoid issues when the vault sits behind a proxy that rewrites the host),
// it keeps the configured base URL and only appends the server-assigned upload
// id taken from the Location.
func resolveTusLocation(baseURL, location string) string {
	if ref, err := url.Parse(location); err == nil {
		trimmed := strings.Trim(ref.Path, "/")
		if trimmed != "" {
			segments := strings.Split(trimmed, "/")
			id := segments[len(segments)-1]
			if id != "" {
				return strings.TrimRight(baseURL, "/") + "/" + id
			}
		}
	}
	// Fallback: resolve the reference against the base URL as-is.
	if base, err := url.Parse(baseURL); err == nil {
		if ref, err := url.Parse(location); err == nil {
			return base.ResolveReference(ref).String()
		}
	}
	return location
}

// tusSidecarDir is the directory where resume state files are kept. It is
// intentionally separate from data/cloud (which is scanned for recordings to
// upload) so the sidecar files are never mistaken for recordings.
func tusSidecarDir() string {
	return "data/tus"
}

func tusSidecarPath(fileName, slot string) string {
	safe := strings.ReplaceAll(fileName, "/", "_")
	safe = strings.ReplaceAll(safe, string(os.PathSeparator), "_")
	return filepath.Join(tusSidecarDir(), safe+"."+slot+".json")
}

// loadTusResumeState returns a previously stored upload URL for the given
// sidecar, but only if it was created against the same vault base URL. Any
// mismatch or read/parse error yields an empty string (start fresh).
func loadTusResumeState(path, baseURL string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var state tusResumeState
	if err := json.Unmarshal(b, &state); err != nil {
		return ""
	}
	if state.UploadURL == "" || state.VaultURI != baseURL {
		return ""
	}
	return state.UploadURL
}

func saveTusResumeState(path string, state tusResumeState) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	b, err := json.Marshal(state)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, b, 0o644)
}

func removeTusResumeState(path string) {
	_ = os.Remove(path)
}

// tusBackoff sleeps for an exponentially increasing duration (capped) between
// resume attempts to avoid hammering a temporarily unavailable vault.
func tusBackoff(attempt int) {
	delay := time.Duration(500*(1<<uint(attempt))) * time.Millisecond
	if delay > 3*time.Second {
		delay = 3 * time.Second
	}
	time.Sleep(delay)
}
