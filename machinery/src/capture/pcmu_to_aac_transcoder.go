package capture

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
	"github.com/kerberos-io/agent/machinery/src/packets"
	"github.com/kerberos-io/agent/machinery/src/video"
)

const (
	defaultPCMUSampleRate = 8000
	defaultPCMUChannels   = 1
)

func PCMUToAACTranscodingAvailable() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

type PCMUToAACTranscoder struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr *bytes.Buffer

	mu          sync.Mutex
	outMu       sync.Mutex
	outBuf      bytes.Buffer
	closed      bool
	stdinClosed bool
	closeOnce   sync.Once
}

func NewPCMUToAACTranscoder(sampleRate int, channels int) (*PCMUToAACTranscoder, error) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, errors.New("PCM_MULAW to AAC transcoding not available: ffmpeg binary not found in PATH")
	}

	if sampleRate <= 0 {
		sampleRate = defaultPCMUSampleRate
	}
	if channels <= 0 {
		channels = defaultPCMUChannels
	}

	cmd := exec.Command(
		ffmpegPath,
		"-hide_banner",
		"-loglevel", "error",
		"-fflags", "+nobuffer",
		"-flags", "low_delay",
		"-f", "mulaw",
		"-ar", intToString(sampleRate),
		"-ac", intToString(channels),
		"-i", "pipe:0",
		"-vn",
		"-ac", "1",
		"-ar", intToString(sampleRate),
		"-c:a", "aac",
		"-profile:a", "aac_low",
		"-b:a", "32k",
		"-f", "adts",
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
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	t := &PCMUToAACTranscoder{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
	}

	go func() {
		buf := make([]byte, 4096)
		for {
			n, readErr := stdout.Read(buf)
			if n > 0 {
				t.outMu.Lock()
				_, _ = t.outBuf.Write(buf[:n])
				t.outMu.Unlock()
			}
			if readErr != nil {
				if readErr != io.EOF {
					log.Log.Warning("capture.pcmu_to_aac: stdout reader stopped: " + readErr.Error())
				}
				return
			}
		}
	}()

	log.Log.Info("capture.pcmu_to_aac: PCM_MULAW -> AAC transcoder initialised (ffmpeg process)")
	return t, nil
}

func (t *PCMUToAACTranscoder) Transcode(mulawData []byte) ([]byte, error) {
	if t == nil || len(mulawData) == 0 {
		return nil, nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil, errors.New("PCM_MULAW to AAC transcoder is closed")
	}
	if t.stdinClosed {
		return nil, errors.New("PCM_MULAW to AAC transcoder input is closed")
	}

	if _, err := t.stdin.Write(mulawData); err != nil {
		return nil, err
	}

	deadline := time.Now().Add(75 * time.Millisecond)
	for {
		data := t.readAvailable()
		if len(data) > 0 {
			return data, nil
		}
		if time.Now().After(deadline) {
			return nil, nil
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (t *PCMUToAACTranscoder) Flush() ([]byte, error) {
	if t == nil {
		return nil, nil
	}

	t.mu.Lock()
	if t.closed {
		defer t.mu.Unlock()
		return t.readAvailable(), nil
	}
	if !t.stdinClosed && t.stdin != nil {
		if err := t.stdin.Close(); err != nil {
			t.mu.Unlock()
			return nil, err
		}
		t.stdinClosed = true
	}
	t.mu.Unlock()

	deadline := time.Now().Add(750 * time.Millisecond)
	previousLen := -1
	stableReads := 0
	for {
		buffered := t.bufferedLen()
		if buffered == previousLen {
			stableReads++
		} else {
			stableReads = 0
			previousLen = buffered
		}

		if stableReads >= 3 || time.Now().After(deadline) {
			break
		}

		time.Sleep(15 * time.Millisecond)
	}

	return t.readAvailable(), nil
}

func (t *PCMUToAACTranscoder) Close() {
	if t == nil {
		return
	}

	t.closeOnce.Do(func() {
		t.mu.Lock()
		t.closed = true
		if t.stdin != nil && !t.stdinClosed {
			_ = t.stdin.Close()
			t.stdinClosed = true
		}
		t.mu.Unlock()

		if t.stdout != nil {
			_ = t.stdout.Close()
		}

		if t.cmd != nil && t.cmd.Process != nil {
			_ = t.cmd.Process.Kill()
			_, _ = t.cmd.Process.Wait()
		}

		if stderr := t.stderrString(); stderr != "" {
			log.Log.Info("capture.pcmu_to_aac: ffmpeg stderr on close: " + stderr)
		}
	})
}

func (t *PCMUToAACTranscoder) readAvailable() []byte {
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

func (t *PCMUToAACTranscoder) bufferedLen() int {
	t.outMu.Lock()
	defer t.outMu.Unlock()
	return t.outBuf.Len()
}

func (t *PCMUToAACTranscoder) stderrString() string {
	if t == nil || t.stderr == nil {
		return ""
	}
	return strings.TrimSpace(t.stderr.String())
}

type recordingAudioWriter struct {
	mp4        *video.MP4
	trackID    uint32
	transcoder *PCMUToAACTranscoder
	lastPTS    uint64
	logPrefix  string
}

func newRecordingAudioWriter(mp4Video *video.MP4, audioCodec string, sampleRate int, channels int, logPrefix string) *recordingAudioWriter {
	writer := &recordingAudioWriter{
		mp4:       mp4Video,
		logPrefix: logPrefix,
	}

	switch audioCodec {
	case "AAC":
		writer.trackID = mp4Video.AddAudioTrack("AAC")
	case "PCM_MULAW":
		if sampleRate <= 0 {
			sampleRate = defaultPCMUSampleRate
		}
		if channels <= 0 {
			channels = defaultPCMUChannels
		}

		if !PCMUToAACTranscodingAvailable() {
			log.Log.Warning(logPrefix + ": ffmpeg not available, skipping PCM_MULAW audio recording.")
			return writer
		}

		transcoder, err := NewPCMUToAACTranscoder(sampleRate, channels)
		if err != nil {
			log.Log.Error(logPrefix + ": failed to create PCM_MULAW to AAC transcoder: " + err.Error())
			return writer
		}

		writer.trackID = mp4Video.AddAudioTrack("AAC")
		writer.transcoder = transcoder
		log.Log.Info(logPrefix + ": recording PCM_MULAW audio as AAC.")
	}

	return writer
}

func (w *recordingAudioWriter) TrackID() uint32 {
	if w == nil {
		return 0
	}
	return w.trackID
}

func (w *recordingAudioWriter) WritePacket(pkt packets.Packet) error {
	if w == nil || w.mp4 == nil || !pkt.IsAudio || w.trackID == 0 {
		return nil
	}

	pts := convertPTS(pkt.TimeLegacy)
	if pts > 0 {
		w.lastPTS = pts
	}

	switch pkt.Codec {
	case "AAC":
		return w.mp4.AddSampleToTrack(w.trackID, pkt.IsKeyFrame, pkt.Data, pts)
	case "PCM_MULAW":
		if w.transcoder == nil {
			return nil
		}

		adts, err := w.transcoder.Transcode(pkt.Data)
		if err != nil {
			return err
		}
		if len(adts) == 0 {
			return nil
		}

		return w.mp4.AddSampleToTrack(w.trackID, false, adts, pts)
	default:
		return nil
	}
}

func (w *recordingAudioWriter) Flush() error {
	if w == nil || w.transcoder == nil || w.mp4 == nil || w.trackID == 0 {
		return nil
	}

	adts, err := w.transcoder.Flush()
	if err != nil {
		return err
	}
	if len(adts) == 0 {
		return nil
	}

	pts := w.lastPTS
	if pts == 0 {
		pts = 1
	}

	return w.mp4.AddSampleToTrack(w.trackID, false, adts, pts)
}

func (w *recordingAudioWriter) Close() {
	if w != nil && w.transcoder != nil {
		w.transcoder.Close()
		w.transcoder = nil
	}
}

func intToString(v int) string {
	return strconv.Itoa(v)
}