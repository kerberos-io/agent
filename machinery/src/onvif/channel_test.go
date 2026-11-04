package onvif

import (
	"fmt"
	"testing"

	xsdonvif "github.com/kerberos-io/onvif/xsd/onvif"
)

func testProfile(token string, source string, ptz bool) xsdonvif.Profile {
	profile := xsdonvif.Profile{Token: xsdonvif.ReferenceToken(token)}
	if source != "" {
		sourceToken := xsdonvif.ReferenceToken(source)
		profile.VideoSourceConfiguration = &xsdonvif.VideoSourceConfiguration{SourceToken: &sourceToken}
	}
	if ptz {
		profile.PTZConfiguration = &xsdonvif.PTZConfiguration{}
	}
	return profile
}

// spe420Profiles mirrors a Hanwha SPE-420: three profiles per channel, token
// suffix -(N-1), listed grouped by profile type rather than by channel.
func spe420Profiles() []xsdonvif.Profile {
	var profiles []xsdonvif.Profile
	for kind := 1; kind <= 3; kind++ {
		for ch := 0; ch < 4; ch++ {
			profiles = append(profiles, testProfile(
				fmt.Sprintf("DefaultProfile-0%d-%d", kind, ch),
				fmt.Sprintf("VideoSourceToken-%d", ch),
				true,
			))
		}
	}
	return profiles
}

func TestSelectChannelProfileMapsEncoderChannels(t *testing.T) {
	profiles := spe420Profiles()
	for channel := 1; channel <= 4; channel++ {
		profile, err := selectChannelProfile(profiles, channel)
		if err != nil {
			t.Fatalf("channel %d: %v", channel, err)
		}
		want := fmt.Sprintf("DefaultProfile-01-%d", channel-1)
		if string(profile.Token) != want {
			t.Fatalf("channel %d: got %s, want %s", channel, profile.Token, want)
		}
	}
}

func TestSelectChannelProfilePrefersProfileWithPTZ(t *testing.T) {
	profiles := []xsdonvif.Profile{
		testProfile("mjpeg", "src-0", false),
		testProfile("main", "src-0", true),
	}
	profile, err := selectChannelProfile(profiles, 1)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Token != "main" {
		t.Fatalf("got %s, want main", profile.Token)
	}
}

func TestSelectChannelProfileFallsBackToFirstProfileOfChannel(t *testing.T) {
	profiles := []xsdonvif.Profile{
		testProfile("a", "src-0", false),
		testProfile("b", "src-1", false),
		testProfile("c", "src-1", false),
	}
	profile, err := selectChannelProfile(profiles, 2)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Token != "b" {
		t.Fatalf("got %s, want b", profile.Token)
	}
}

func TestSelectChannelProfileSkipsProfilesWithoutVideoSource(t *testing.T) {
	profiles := []xsdonvif.Profile{
		testProfile("audio-only", "", false),
		testProfile("video", "src-0", false),
	}
	profile, err := selectChannelProfile(profiles, 1)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Token != "video" {
		t.Fatalf("got %s, want video", profile.Token)
	}
}

func TestSelectChannelProfileRejectsUnknownChannel(t *testing.T) {
	if _, err := selectChannelProfile(spe420Profiles(), 5); err == nil {
		t.Fatal("expected error for channel beyond the device's video sources")
	}
	if _, err := selectChannelProfile(spe420Profiles(), 0); err == nil {
		t.Fatal("expected error for channel 0")
	}
	if _, err := selectChannelProfile(nil, 1); err == nil {
		t.Fatal("expected error when the device has no profiles")
	}
}
