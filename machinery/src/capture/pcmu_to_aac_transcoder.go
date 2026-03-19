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

type audioToAACTranscoder interface {
	Transcode([]byte) ([]byte, error)
	Flush() ([]byte, error)
	Close()
}

func PCMUToAACTranscodingAvailable() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

type ffmpegToAACTranscoder struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr *bytes.Buffer

	mu          sync.Mutex
	outMu       sync.Mutex
	outBuf      bytes.Buffer
	adtsBuf     []byte
	closed      bool
	stdinClosed bool
	closeOnce   sync.Once
	stdoutDone  chan struct{}
	waitDone    chan struct{}
	waitErr     error
}

func newFFmpegToAACTranscoder(inputFormat string, sampleRate int, channels int) (*ffmpegToAACTranscoder, error) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, errors.New("audio to AAC transcoding not available: ffmpeg binary not found in PATH")
	}

	cmd := exec.Command(
		ffmpegPath,
		"-hide_banner",
		"-loglevel", "error",
		"-fflags", "+nobuffer",
		"-flags", "low_delay",
		"-f", inputFormat,
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

	t := &ffmpegToAACTranscoder{
		cmd:        cmd,
		stdin:      stdin,
		stdout:     stdout,
		stderr:     stderr,
		stdoutDone: make(chan struct{}),
		waitDone:   make(chan struct{}),
	}

	go func() {
		defer close(t.stdoutDone)
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

	go func() {
		t.waitErr = cmd.Wait()
		close(t.waitDone)
	}()

	log.Log.Info("capture.audio_to_aac: " + strings.ToUpper(inputFormat) + " -> AAC transcoder initialised (ffmpeg process)")
	return t, nil
}

func NewPCMUToAACTranscoder(sampleRate int, channels int) (*ffmpegToAACTranscoder, error) {
	if sampleRate <= 0 {
		sampleRate = defaultPCMUSampleRate
	}
	if channels <= 0 {
		channels = defaultPCMUChannels
	}

	return newFFmpegToAACTranscoder("mulaw", sampleRate, channels)
}

func NewLPCMToAACTranscoder(sampleRate int, channels int, bitDepth int) (*ffmpegToAACTranscoder, error) {
	inputFormat, err := lpcmFFmpegInputFormat(bitDepth)
	if err != nil {
		return nil, err
	}
	if sampleRate <= 0 {
		return nil, errors.New("LPCM to AAC transcoding requires a valid sample rate")
	}
	if channels <= 0 {
		return nil, errors.New("LPCM to AAC transcoding requires a valid channel count")
	}

	return newFFmpegToAACTranscoder(inputFormat, sampleRate, channels)
}

func (t *ffmpegToAACTranscoder) Transcode(input []byte) ([]byte, error) {
	if t == nil || len(input) == 0 {
		return nil, nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil, errors.New("audio to AAC transcoder is closed")
	}
	if t.stdinClosed {
		return nil, errors.New("audio to AAC transcoder input is closed")
	}

	if _, err := t.stdin.Write(input); err != nil {
		return nil, err
	}

	// Do not block the recording loop waiting for the encoder to emit output.
	// FFmpeg can buffer for a while, and polling here per RTP packet causes the
	// recorder to fall behind real time and drop trailing media before close.
	return t.readAvailable(), nil
}

func (t *ffmpegToAACTranscoder) Flush() ([]byte, error) {
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

	processExited := false
	readerFinished := false
	deadline := time.Now().Add(15 * time.Second)
	timedOut := false
	for !processExited || !readerFinished {
		if !processExited {
			select {
			case <-t.waitDone:
				processExited = true
			default:
			}
		}
		if !readerFinished {
			select {
			case <-t.stdoutDone:
				readerFinished = true
			default:
			}
		}

		if processExited && readerFinished {
			break
		}
		if time.Now().After(deadline) {
			timedOut = true
			break
		}

		time.Sleep(15 * time.Millisecond)
	}

	if timedOut {
		log.Log.Warning("capture.audio_to_aac: flush timed out before ffmpeg fully drained (process_exited=" + strconv.FormatBool(processExited) + ", stdout_done=" + strconv.FormatBool(readerFinished) + ", buffered=" + intToString(t.bufferedLen()) + ")")
	}

	if processExited && t.waitErr != nil {
		return t.readAvailable(), t.waitErr
	}

	return t.readAvailable(), nil
}

func (t *ffmpegToAACTranscoder) Close() {
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

		processExited := false
		waitTimeout := 250 * time.Millisecond
		if t.stdinClosed {
			waitTimeout = 2 * time.Second
		}
		select {
		case <-t.waitDone:
			processExited = true
		case <-time.After(waitTimeout):
		}

		if !processExited {
			if t.stdout != nil {
				_ = t.stdout.Close()
			}
			if t.cmd != nil && t.cmd.Process != nil {
				_ = t.cmd.Process.Kill()
				<-t.waitDone
			}
		}

		if stderr := t.stderrString(); stderr != "" {
			log.Log.Info("capture.audio_to_aac: ffmpeg stderr on close: " + stderr)
		}
	})
}

func (t *ffmpegToAACTranscoder) readAvailable() []byte {
	t.outMu.Lock()
	defer t.outMu.Unlock()

	if t.outBuf.Len() > 0 {
		t.adtsBuf = append(t.adtsBuf, t.outBuf.Bytes()...)
		t.outBuf.Reset()
	}

	return drainCompleteADTSFrames(&t.adtsBuf)
}

func (t *ffmpegToAACTranscoder) bufferedLen() int {
	t.outMu.Lock()
	defer t.outMu.Unlock()
	return t.outBuf.Len()
}

func (t *ffmpegToAACTranscoder) stderrString() string {
	if t == nil || t.stderr == nil {
		return ""
	}
	return strings.TrimSpace(t.stderr.String())
}

func lpcmFFmpegInputFormat(bitDepth int) (string, error) {
	switch bitDepth {
	case 8:
		return "u8", nil
	case 16:
		return "s16be", nil
	case 24:
		return "s24be", nil
	default:
		return "", errors.New("unsupported LPCM bit depth: " + intToString(bitDepth))
	}
}

type recordingAudioWriter struct {
	mp4        *video.MP4
	trackID    uint32
	transcoder audioToAACTranscoder
	lastPTS    uint64
	logPrefix  string

	transcodedSampleRate int
	aacBasePTS           uint64
	aacFrameCursor       uint64
	aacClockStarted      bool
	loggedAACParams      bool
}

func newRecordingAudioWriter(mp4Video *video.MP4, audioCodec string, sampleRate int, channels int, bitDepth int, logPrefix string) *recordingAudioWriter {
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
		writer.transcodedSampleRate = sampleRate
		log.Log.Info(logPrefix + ": recording PCM_MULAW audio as AAC (input_rate=" + intToString(sampleRate) + ", channels=" + intToString(channels) + ").")
	case "LPCM":
		transcoder, err := NewLPCMToAACTranscoder(sampleRate, channels, bitDepth)
		if err != nil {
			log.Log.Error(logPrefix + ": failed to create LPCM to AAC transcoder: " + err.Error())
			return writer
		}

		writer.trackID = mp4Video.AddAudioTrack("AAC")
		writer.transcoder = transcoder
		writer.transcodedSampleRate = sampleRate
		log.Log.Info(logPrefix + ": recording LPCM audio as AAC (input_rate=" + intToString(sampleRate) + ", channels=" + intToString(channels) + ", bit_depth=" + intToString(bitDepth) + ").")
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
	case "PCM_MULAW", "LPCM":
		if w.transcoder == nil {
			return nil
		}
		if !w.aacClockStarted {
			w.aacBasePTS = pts
			w.aacClockStarted = true
		}

		adts, err := w.transcoder.Transcode(pkt.Data)
		if err != nil {
			return err
		}
		if len(adts) == 0 {
			return nil
		}

		return w.writeTranscodedADTS(adts)
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

	return w.writeTranscodedADTS(adts)
}

func (w *recordingAudioWriter) Close() {
	if w != nil && w.transcoder != nil {
		w.transcoder.Close()
		w.transcoder = nil
	}
}

func (w *recordingAudioWriter) writeTranscodedADTS(adts []byte) error {
	if w == nil || w.mp4 == nil || w.trackID == 0 || len(adts) == 0 {
		return nil
	}
	if w.transcodedSampleRate <= 0 {
		return errors.New("transcoded AAC sample rate is not set")
	}

	var writeErr error
	video.SplitAACFrame(adts, func(started bool, aac []byte) {
		if writeErr != nil || len(aac) < 7 {
			return
		}

		if !w.loggedAACParams {
			log.Log.Info(w.logPrefix + ": first AAC frame parameters (aac_rate=" + intToString(int(video.AACSampleRateFromADTS(aac))) + ", channels=" + intToString(int(video.AACChannelCountFromADTS(aac))) + ", samples_per_frame=" + intToString(aacSamplesPerFrame(aac)) + ").")
			w.loggedAACParams = true
		}

		pts := w.transcodedPTS()
		if err := w.mp4.AddSampleToTrack(w.trackID, false, aac, pts); err != nil {
			writeErr = err
			return
		}

		w.lastPTS = pts
		w.aacFrameCursor += uint64(aacSamplesPerFrame(aac))
	})

	return writeErr
}

func (w *recordingAudioWriter) transcodedPTS() uint64 {
	if w == nil {
		return 0
	}
	if !w.aacClockStarted {
		w.aacClockStarted = true
	}
	return w.aacBasePTS + (w.aacFrameCursor*1000)/uint64(w.transcodedSampleRate)
}

func aacSamplesPerFrame(aac []byte) int {
	if len(aac) < 7 {
		return 1024
	}
	rawBlocks := int(aac[6]&0x03) + 1
	return rawBlocks * 1024
}

func intToString(v int) string {
	return strconv.Itoa(v)
}

func drainCompleteADTSFrames(buffer *[]byte) []byte {
	if buffer == nil || len(*buffer) == 0 {
		return nil
	}

	data := *buffer
	start := video.FindSyncword(data, 0)
	if start < 0 {
		// Keep the tail in case the syncword is split across reads.
		if len(data) > 1 {
			*buffer = append([]byte{}, data[len(data)-1:]...)
		}
		return nil
	}
	if start > 0 {
		data = data[start:]
	}

	var out bytes.Buffer
	offset := 0
	for {
		if len(data[offset:]) < 7 {
			break
		}

		var adts video.ADTS_Frame_Header
		adts.Decode(data[offset:])
		frameLen := int(adts.Variable_Header.Frame_length)
		if frameLen < 7 {
			next := video.FindSyncword(data, offset+1)
			if next < 0 {
				break
			}
			offset = next
			continue
		}
		if offset+frameLen > len(data) {
			break
		}

		_, _ = out.Write(data[offset : offset+frameLen])
		offset += frameLen

		next := video.FindSyncword(data, offset)
		if next < 0 {
			break
		}
		if next > offset {
			offset = next
		}
	}

	if offset < len(data) {
		*buffer = append([]byte{}, data[offset:]...)
	} else {
		*buffer = nil
	}

	if out.Len() == 0 {
		return nil
	}

	return out.Bytes()
}
