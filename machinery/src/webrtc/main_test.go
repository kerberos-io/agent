package webrtc

import "testing"

func TestBuildICEServersOmitsEmptyURLs(t *testing.T) {
	webRTC := CreateWebRTC("camera", []string{""}, []string{""}, "", "")

	iceServers := buildICEServers(*webRTC)

	if len(iceServers) != 0 {
		t.Fatalf("buildICEServers() returned %d servers, want 0", len(iceServers))
	}
}

func TestBuildICEServersIncludesConfiguredURLs(t *testing.T) {
	webRTC := CreateWebRTC(
		"camera",
		[]string{"", " stun:turn-fra1.kerberos.io:3478 "},
		[]string{" turn:turn-fra1.kerberos.io:3478 "},
		"username",
		"credential",
	)

	iceServers := buildICEServers(*webRTC)

	if len(iceServers) != 2 {
		t.Fatalf("buildICEServers() returned %d servers, want 2", len(iceServers))
	}
	if got := iceServers[0].URLs[0]; got != "stun:turn-fra1.kerberos.io:3478" {
		t.Fatalf("STUN URL = %q, want trimmed URL", got)
	}
	if got := iceServers[1].URLs[0]; got != "turn:turn-fra1.kerberos.io:3478" {
		t.Fatalf("TURN URL = %q, want trimmed URL", got)
	}
	if iceServers[1].Username != "username" || iceServers[1].Credential != "credential" {
		t.Fatal("TURN credentials were not preserved")
	}
}
