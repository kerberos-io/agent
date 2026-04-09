// AAC transcoding fallback that uses the ffmpeg binary at runtime.
// Build with -tags ffmpeg to use the in-process CGO implementation instead.
//
//go:build !ffmpeg

package webrtc

import (
	"bytes"
	"errors"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kerberos-io/agent/machinery/src/log"
)

// AACTranscodingAvailable reports whether AAC→PCMU transcoding
// is available in the current runtime.
func AACTranscodingAvailable() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

// AACTranscoder uses an ffmpeg subprocess to convert ADTS AAC to raw PCMU.
type AACTranscoder struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	stderrBuf bytes.Buffer

	mu        sync.Mutex
	outMu     sync.Mutex
	outBuf    bytes.Buffer
	closed    bool
	closeOnce sync.Once
}

// NewAACTranscoder creates a runtime ffmpeg-based transcoder.
func NewAACTranscoder() (*AACTranscoder, error) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, errors.New("AAC transcoding not available: ffmpeg binary not found in PATH")
	}
	log.Log.Info("webrtc.aac_transcoder: using ffmpeg binary at " + ffmpegPath)

	cmd := exec.Command(
		ffmpegPath,
		"-hide_banner",
		"-loglevel", "error",
		"-fflags", "+nobuffer",
		"-flags", "low_delay",
		"-f", "aac",
		"-i", "pipe:0",
		"-vn",
		"-ac", "1",
		"-ar", "8000",
		"-acodec", "pcm_mulaw",
		"-f", "mulaw",
		"pipe:1",
	)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = &bytes.Buffer{}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	t := &AACTranscoder{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
	}
	if stderrBuf, ok := cmd.Stderr.(*bytes.Buffer); ok {
		t.stderrBuf = *stderrBuf
	}

	go func() {
		buf := make([]byte, 4096)
		for {
			n, readErr := stdout.Read(buf)
			if n > 0 {
				t.outMu.Lock()
				_, _ = t.outBuf.Write(buf[:n])
				buffered := t.outBuf.Len()
				t.outMu.Unlock()
				if buffered <= 8192 || buffered%16000 == 0 {
					log.Log.Debug("webrtc.aac_transcoder: ffmpeg produced PCMU bytes, buffered=" + strconv.Itoa(buffered))
				}
			}
			if readErr != nil {
				if readErr != io.EOF {
					log.Log.Warning("webrtc.aac_transcoder: stdout reader stopped: " + readErr.Error())
				}
				return
			}
		}
	}()

	log.Log.Info("webrtc.aac_transcoder: AAC → PCMU transcoder initialised (ffmpeg process)")
	return t, nil
}

// Transcode writes ADTS AAC to ffmpeg and returns any PCMU bytes produced.
func (t *AACTranscoder) Transcode(adtsData []byte) ([]byte, error) {
	if t == nil || len(adtsData) == 0 {
		return nil, nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil, errors.New("AAC transcoder is closed")
	}

	if _, err := t.stdin.Write(adtsData); err != nil {
		return nil, err
	}
	if len(adtsData) <= 512 || len(adtsData)%1024 == 0 {
		log.Log.Debug("webrtc.aac_transcoder: wrote AAC bytes to ffmpeg, input=" + strconv.Itoa(len(adtsData)))
	}

	deadline := time.Now().Add(75 * time.Millisecond)
	for {
		data := t.readAvailable()
		if len(data) > 0 {
			log.Log.Debug("webrtc.aac_transcoder: returning PCMU bytes=" + strconv.Itoa(len(data)))
			return data, nil
		}

		if time.Now().After(deadline) {
			if stderr := t.stderrString(); stderr != "" {
				log.Log.Warning("webrtc.aac_transcoder: no output before deadline, ffmpeg stderr: " + stderr)
			} else {
				log.Log.Debug("webrtc.aac_transcoder: no PCMU output before deadline")
			}
			return nil, nil
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (t *AACTranscoder) readAvailable() []byte {
	t.outMu.Lock()
	defer t.outMu.Unlock()

	if t.outBuf.Len() == 0 {
		return nil
	}

	out := make([]byte, t.outBuf.Len())
	copy(out, t.outBuf.Bytes())
	t.outBuf.Reset()
	return out
}

func (t *AACTranscoder) stderrString() string {
	if t == nil {
		return ""
	}
	if stderrBuf, ok := t.cmd.Stderr.(*bytes.Buffer); ok {
		return strings.TrimSpace(stderrBuf.String())
	}
	return strings.TrimSpace(t.stderrBuf.String())
}

// Close stops the ffmpeg subprocess.
func (t *AACTranscoder) Close() {
	if t == nil {
		return
	}

	t.closeOnce.Do(func() {
		t.mu.Lock()
		t.closed = true
		if t.stdin != nil {
			_ = t.stdin.Close()
		}
		t.mu.Unlock()

		if t.stdout != nil {
			_ = t.stdout.Close()
		}

		if t.cmd != nil {
			_ = t.cmd.Process.Kill()
			_, _ = t.cmd.Process.Wait()
			if stderr := t.stderrString(); stderr != "" {
				log.Log.Info("webrtc.aac_transcoder: ffmpeg stderr on close: " + stderr)
			}
		}
	})
}
