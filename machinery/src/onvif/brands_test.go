package onvif

import (
	"bufio"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

// mockRTSPServer starts a TCP listener that answers RTSP DESCRIBE requests. For
// each incoming request it extracts the path and calls respond(path) to obtain
// the numeric status code and optional auth realm to return. It returns the
// listener host, port and a cleanup function.
func mockRTSPServer(t *testing.T, respond func(path string) (int, string)) (string, int, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock RTSP server: %v", err)
	}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_ = c.SetDeadline(time.Now().Add(2 * time.Second))
				reader := bufio.NewReader(c)
				line, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				path := ""
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					url := fields[1]
					url = strings.TrimPrefix(url, "rtsp://")
					if idx := strings.Index(url, "/"); idx >= 0 {
						path = url[idx:]
					}
				}
				status, realm := respond(path)
				reason := map[int]string{200: "OK", 401: "Unauthorized", 404: "Not Found"}[status]
				response := "RTSP/1.0 " + strconv.Itoa(status) + " " + reason + "\r\nCSeq: 1\r\n"
				if realm != "" {
					response += "WWW-Authenticate: Digest realm=\"" + realm + "\", nonce=\"abc\"\r\n"
				}
				response += "\r\n"
				_, _ = c.Write([]byte(response))
			}(conn)
		}
	}()

	host, portStr, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(portStr)
	return host, port, func() { listener.Close() }
}

// TestGuessRTSPStreams_DiscriminatingHikvision verifies that a device which
// distinguishes valid from invalid paths (returning 401 only for the Hikvision
// path) is correctly identified as Hikvision with a confirmed main/sub stream.
func TestGuessRTSPStreams_DiscriminatingHikvision(t *testing.T) {
	host, port, cleanup := mockRTSPServer(t, func(path string) (int, string) {
		if strings.HasPrefix(path, "/Streaming/Channels/") {
			return 401, "" // valid path, needs auth
		}
		return 404, "" // everything else is unknown -> device discriminates
	})
	defer cleanup()

	brand, _, streams := guessRTSPStreams(host, port, "", nil, 2*time.Second)
	if brand != "Hikvision" {
		t.Fatalf("expected brand Hikvision, got %q", brand)
	}
	if len(streams) == 0 || !streams[0].Verified {
		t.Fatalf("expected a verified main stream, got %+v", streams)
	}
	if !streams[0].RequiresAuth {
		t.Errorf("expected main stream to require auth")
	}
	if streams[0].Path != "/Streaming/Channels/101" {
		t.Errorf("expected main path /Streaming/Channels/101, got %q", streams[0].Path)
	}
}

// TestGuessRTSPStreams_ChallengesEverything verifies that a device which returns
// 401 for *any* path (including a bogus one) does NOT get mis-detected via path
// probing, and instead falls back to the port hint (Dahua control port 37777)
// with unverified suggestions.
func TestGuessRTSPStreams_ChallengesEverything(t *testing.T) {
	host, port, cleanup := mockRTSPServer(t, func(path string) (int, string) {
		return 401, "" // challenges auth before checking the path, no realm
	})
	defer cleanup()

	brand, _, streams := guessRTSPStreams(host, port, "", []int{37777}, 2*time.Second)
	if brand != "Dahua" {
		t.Fatalf("expected fallback brand Dahua from port hint, got %q", brand)
	}
	if len(streams) == 0 {
		t.Fatalf("expected suggested streams, got none")
	}
	if streams[0].Verified {
		t.Errorf("expected unverified suggestion for a non-discriminating device")
	}
	if streams[0].Path != "/cam/realmonitor?channel=1&subtype=0" {
		t.Errorf("expected Dahua main path, got %q", streams[0].Path)
	}
}

// TestGuessRTSPStreams_RealmDetectsHikvision verifies that a device which
// challenges auth for every path (so path probing cannot help) is still
// identified from its RTSP auth realm, and the model code is extracted.
func TestGuessRTSPStreams_RealmDetectsHikvision(t *testing.T) {
	host, port, cleanup := mockRTSPServer(t, func(path string) (int, string) {
		return 401, "IP Camera(E3669)" // Hikvision realm signature, 401 for all paths
	})
	defer cleanup()

	brand, model, streams := guessRTSPStreams(host, port, "", nil, 2*time.Second)
	if brand != "Hikvision" {
		t.Fatalf("expected brand Hikvision from realm, got %q", brand)
	}
	if model != "E3669" {
		t.Errorf("expected model E3669 from realm, got %q", model)
	}
	if len(streams) == 0 || streams[0].Path != "/Streaming/Channels/101" {
		t.Fatalf("expected Hikvision default main path, got %+v", streams)
	}
	if !streams[0].RequiresAuth {
		t.Errorf("expected the suggestion to be marked auth-required")
	}
}

// TestGuessRTSPStreams_RealmDetectsDahua verifies Dahua detection from its
// "Login to ..." realm.
func TestGuessRTSPStreams_RealmDetectsDahua(t *testing.T) {
	host, port, cleanup := mockRTSPServer(t, func(path string) (int, string) {
		return 401, "Login to 5df61a6057b10cc99d471769516d3c11"
	})
	defer cleanup()

	brand, _, streams := guessRTSPStreams(host, port, "", nil, 2*time.Second)
	if brand != "Dahua" {
		t.Fatalf("expected brand Dahua from realm, got %q", brand)
	}
	if len(streams) == 0 || streams[0].Path != "/cam/realmonitor?channel=1&subtype=0" {
		t.Fatalf("expected Dahua default main path, got %+v", streams)
	}
}

// TestGuessRTSPStreams_UnknownFallsBackToGeneric verifies that an unknown device
// (discriminating but matching no brand) yields generic suggestions.
func TestGuessRTSPStreams_UnknownFallsBackToGeneric(t *testing.T) {
	host, port, cleanup := mockRTSPServer(t, func(path string) (int, string) {
		return 404, "" // discriminates, but nothing matches
	})
	defer cleanup()

	brand, _, streams := guessRTSPStreams(host, port, "", nil, 2*time.Second)
	if brand != "" {
		t.Fatalf("expected no detected brand, got %q", brand)
	}
	if len(streams) == 0 || streams[0].Brand != "Generic" {
		t.Fatalf("expected generic suggestions, got %+v", streams)
	}
}
