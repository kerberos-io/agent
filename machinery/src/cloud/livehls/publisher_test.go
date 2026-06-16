package livehls

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/kerberos-io/agent/machinery/src/packets"
	"github.com/kerberos-io/agent/machinery/src/video"
)

// captured records one received upload for assertions.
type captured struct {
	path        string
	method      string
	contentType string
	device      string
	session     string
	name        string
	sequence    string
	duration    string
	hubPublic   string
	hubPrivate  string
	region      string
	body        []byte
}

// newCapturingServer returns an httptest server that records every upload and
// replies with the given status code.
func newCapturingServer(t *testing.T, status int) (*httptest.Server, *[]captured, *sync.Mutex) {
	t.Helper()
	var mu sync.Mutex
	var got []captured
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, captured{
			path:        r.URL.Path,
			method:      r.Method,
			contentType: r.Header.Get("Content-Type"),
			device:      r.Header.Get(headerStorageDevice),
			session:     r.Header.Get(headerLiveSession),
			name:        r.Header.Get(headerLiveName),
			sequence:    r.Header.Get(headerLiveSequence),
			duration:    r.Header.Get(headerLiveDuration),
			hubPublic:   r.Header.Get(headerHubPublicKey),
			hubPrivate:  r.Header.Get(headerHubPrivateKey),
			region:      r.Header.Get(headerHubRegion),
			body:        body,
		})
		mu.Unlock()
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv, &got, &mu
}

func testPublisher(hubURI string) *Publisher {
	return NewPublisher(PublisherConfig{
		HubURI:        hubURI,
		HubKey:        "pub-key",
		HubPrivateKey: "priv-key",
		Region:        "eu-west",
		DeviceKey:     "cam-1",
		Timeout:       2 * time.Second,
	})
}

func TestPublisherPublishInitSendsContractHeaders(t *testing.T) {
	srv, got, mu := newCapturingServer(t, http.StatusOK)
	p := testPublisher(srv.URL)

	if err := p.PublishInit(context.Background(), "sess-1", []byte("INITBYTES")); err != nil {
		t.Fatalf("PublishInit: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(*got) != 1 {
		t.Fatalf("server received %d requests, want 1", len(*got))
	}
	c := (*got)[0]
	if c.method != http.MethodPost {
		t.Errorf("method=%s, want POST", c.method)
	}
	if c.path != liveIngestPath {
		t.Errorf("path=%s, want %s", c.path, liveIngestPath)
	}
	if c.contentType != contentTypeInit {
		t.Errorf("content-type=%s, want %s", c.contentType, contentTypeInit)
	}
	if c.device != "cam-1" {
		t.Errorf("device=%s, want cam-1", c.device)
	}
	if c.session != "sess-1" {
		t.Errorf("session=%s, want sess-1", c.session)
	}
	if c.name != initObjectName {
		t.Errorf("name=%s, want %s", c.name, initObjectName)
	}
	if c.hubPublic != "pub-key" || c.hubPrivate != "priv-key" || c.region != "eu-west" {
		t.Errorf("auth headers wrong: pub=%q priv=%q region=%q", c.hubPublic, c.hubPrivate, c.region)
	}
	if string(c.body) != "INITBYTES" {
		t.Errorf("body=%q, want INITBYTES", string(c.body))
	}
	// init must NOT carry segment-only headers.
	if c.sequence != "" || c.duration != "" {
		t.Errorf("init should not send sequence/duration, got seq=%q dur=%q", c.sequence, c.duration)
	}
}

func TestPublisherPublishSegmentSendsSequenceAndDuration(t *testing.T) {
	srv, got, mu := newCapturingServer(t, http.StatusOK)
	p := testPublisher(srv.URL)

	seg := video.LiveSegment{SequenceNumber: 7, DurationMs: 1960, Data: []byte("SEGMENT")}
	if err := p.PublishSegment(context.Background(), "sess-9", seg); err != nil {
		t.Fatalf("PublishSegment: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	c := (*got)[0]
	if c.contentType != contentTypeSegment {
		t.Errorf("content-type=%s, want %s", c.contentType, contentTypeSegment)
	}
	if c.name != "seg-7.m4s" {
		t.Errorf("name=%s, want seg-7.m4s", c.name)
	}
	if c.sequence != "7" {
		t.Errorf("sequence=%s, want 7", c.sequence)
	}
	if c.duration != "1960" {
		t.Errorf("duration=%s, want 1960", c.duration)
	}
	if string(c.body) != "SEGMENT" {
		t.Errorf("body=%q, want SEGMENT", string(c.body))
	}
}

func TestPublisherReturnsErrorOnNon2xx(t *testing.T) {
	srv, _, _ := newCapturingServer(t, http.StatusInternalServerError)
	p := testPublisher(srv.URL)

	err := p.PublishSegment(context.Background(), "s", video.LiveSegment{SequenceNumber: 1, Data: []byte("x")})
	if err == nil {
		t.Fatal("expected an error on 500 response")
	}
}

func TestPublisherErrorsWithoutHubURI(t *testing.T) {
	p := NewPublisher(PublisherConfig{DeviceKey: "cam"})
	if err := p.PublishInit(context.Background(), "s", []byte("x")); err == nil {
		t.Fatal("expected error when HubURI is empty")
	}
}

// makeAnnexBVideoPacket builds a synthetic capture packet carrying one Annex B
// H.264 access unit at the given decode time (ms).
func makeAnnexBVideoPacket(isKey bool, timeMs int64) packets.Packet {
	nalType := byte(0x01)
	if isKey {
		nalType = 0x65
	}
	data := []byte{0x00, 0x00, 0x00, 0x01, nalType}
	for i := 0; i < 80; i++ {
		data = append(data, byte(i))
	}
	return packets.Packet{
		IsVideo:    true,
		IsKeyFrame: isKey,
		Codec:      "H264",
		Data:       data,
		TimeLegacy: time.Duration(timeMs) * time.Millisecond,
	}
}

func TestSessionShipsInitThenSegmentsAndFiresReady(t *testing.T) {
	srv, got, mu := newCapturingServer(t, http.StatusOK)
	p := testPublisher(srv.URL)

	sess := NewSession(p, SessionOptions{
		Codec:           "H264",
		SPSNALUs:        [][]byte{liveTestSPSForSession()},
		PPSNALUs:        [][]byte{{0x68, 0xce, 0x38, 0x80}},
		Width:           640,
		Height:          480,
		TargetSegmentMs: 2000,
	})

	var readyCalls int
	var readySession string
	sess.SetOnReady(func(id string) {
		readyCalls++
		readySession = id
	})

	// 4 GOPs of 25 frames @ 40ms = 1s GOPs => with 2s target, 2 segments emitted
	// during streaming and a final one on Close.
	const gopFrames, gops = 25, 4
	for i := 0; i < gopFrames*gops; i++ {
		isKey := i%gopFrames == 0
		pkt := makeAnnexBVideoPacket(isKey, int64(i*40))
		if err := sess.WritePacket(pkt); err != nil {
			t.Fatalf("WritePacket(%d): %v", i, err)
		}
	}
	// A non-video packet must be ignored.
	if err := sess.WritePacket(packets.Packet{IsAudio: true, Data: []byte{1, 2, 3}}); err != nil {
		t.Fatalf("WritePacket(audio): %v", err)
	}
	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	var initCount, segCount int
	for _, c := range *got {
		if c.name == initObjectName {
			initCount++
			if string(c.body[4:8]) != "ftyp" {
				t.Errorf("init body is not an ftyp box: % x", c.body[:12])
			}
		} else {
			segCount++
			if c.session != sess.SessionID() {
				t.Errorf("segment session=%s, want %s", c.session, sess.SessionID())
			}
		}
	}
	if initCount != 1 {
		t.Errorf("init uploaded %d times, want exactly 1", initCount)
	}
	if segCount < 2 {
		t.Errorf("got %d segment uploads, want >= 2", segCount)
	}
	if readyCalls != 1 {
		t.Errorf("OnReady fired %d times, want exactly 1", readyCalls)
	}
	if readySession != sess.SessionID() {
		t.Errorf("OnReady session=%s, want %s", readySession, sess.SessionID())
	}
}

func TestSessionRetriesInitWhenFirstAttemptFails(t *testing.T) {
	// Server fails the first N requests, then succeeds. This proves init is
	// re-attempted (not dropped) so the session can still establish.
	var mu sync.Mutex
	var inits, segs int
	failFirst := 1
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		name := r.Header.Get(headerLiveName)
		if name == initObjectName {
			inits++
			if inits <= failFirst {
				w.WriteHeader(http.StatusBadGateway)
				return
			}
		} else {
			segs++
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	sess := NewSession(testPublisher(srv.URL), SessionOptions{
		Codec:    "H264",
		SPSNALUs: [][]byte{liveTestSPSForSession()},
		PPSNALUs: [][]byte{{0x68, 0xce, 0x38, 0x80}},
		Width:    640,
		Height:   480,
	})

	var ready int
	sess.SetOnReady(func(string) { ready++ })

	for i := 0; i < 60; i++ {
		isKey := i%25 == 0
		if err := sess.WritePacket(makeAnnexBVideoPacket(isKey, int64(i*40))); err != nil {
			t.Fatalf("WritePacket(%d): %v", i, err)
		}
	}
	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if inits < 2 {
		t.Errorf("init attempted %d times, want >= 2 (first failed then retried)", inits)
	}
	if segs < 1 {
		t.Errorf("no segments delivered after init recovered (segs=%d)", segs)
	}
	if ready != 1 {
		t.Errorf("OnReady fired %d times, want 1", ready)
	}
}

// liveTestSPSForSession is the known-good baseline SPS reused across tests.
func liveTestSPSForSession() []byte {
	return []byte{0x67, 0x42, 0xc0, 0x1e, 0xd9, 0x00, 0xa0, 0x47, 0xfe, 0xc8}
}
