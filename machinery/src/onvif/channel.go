package onvif

import (
	"errors"
	"fmt"
	"io"

	"github.com/kerberos-io/onvif"
	"github.com/kerberos-io/onvif/media"
	xsdonvif "github.com/kerberos-io/onvif/xsd/onvif"
)

// Multi-channel devices (encoders, NVRs) expose several media profiles per
// video input, e.g. a Hanwha SPE-420 returns DefaultProfile-01-0..03-0 for
// channel 1 and DefaultProfile-01-1..03-1 for channel 2. PTZ, presets and
// focus must be addressed with a profile (or video source) of the channel the
// camera is wired to, otherwise commands drive the wrong camera.
//
// A channel is 1-based, counted in the order the device first lists each video
// source in GetProfiles. Channel 0 (unset) keeps the historic behaviour of
// using the first profile, so single-sensor cameras need no configuration.

// GetTokenFromChannel returns the media profile token to use for PTZ commands
// on the given 1-based channel.
func GetTokenFromChannel(device *onvif.Device, channel int) (xsdonvif.ReferenceToken, error) {
	if channel <= 0 {
		return GetTokenFromProfile(device, 0)
	}
	profiles, err := getMediaProfiles(device)
	if err != nil {
		return "", err
	}
	profile, err := selectChannelProfile(profiles, channel)
	if err != nil {
		return "", err
	}
	return profile.Token, nil
}

// GetVideoSourceTokenFromChannel returns the video source token of the given
// 1-based channel, as used by the imaging (focus) service.
func GetVideoSourceTokenFromChannel(device *onvif.Device, channel int) (xsdonvif.ReferenceToken, error) {
	if channel <= 0 {
		return GetVideoSourceTokenFromProfile(device, 0)
	}
	profiles, err := getMediaProfiles(device)
	if err != nil {
		return "", err
	}
	profile, err := selectChannelProfile(profiles, channel)
	if err != nil {
		return "", err
	}
	return *profile.VideoSourceConfiguration.SourceToken, nil
}

func getMediaProfiles(device *onvif.Device) ([]xsdonvif.Profile, error) {
	resp, err := device.CallMethod(media.GetProfiles{})
	if err != nil {
		if resp != nil {
			resp.Body.Close()
		}
		return nil, err
	}
	b, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}
	decodedXML, et, err := getXMLNode(string(b), "GetProfilesResponse")
	if err != nil {
		return nil, err
	}
	var profilesResp media.GetProfilesResponse
	if err := decodedXML.DecodeElement(&profilesResp, et); err != nil {
		return nil, err
	}
	return profilesResp.Profiles, nil
}

// selectChannelProfile groups profiles by video source and returns a profile
// of the channel-th source (1-based). Within a channel it prefers the first
// profile that has a PTZ configuration attached.
func selectChannelProfile(profiles []xsdonvif.Profile, channel int) (xsdonvif.Profile, error) {
	if channel <= 0 {
		return xsdonvif.Profile{}, errors.New("onvif channel must be 1 or higher")
	}

	var sources []xsdonvif.ReferenceToken
	bySource := map[xsdonvif.ReferenceToken][]xsdonvif.Profile{}
	for _, profile := range profiles {
		if profile.VideoSourceConfiguration == nil || profile.VideoSourceConfiguration.SourceToken == nil {
			continue
		}
		source := *profile.VideoSourceConfiguration.SourceToken
		if _, seen := bySource[source]; !seen {
			sources = append(sources, source)
		}
		bySource[source] = append(bySource[source], profile)
	}

	if channel > len(sources) {
		return xsdonvif.Profile{}, fmt.Errorf("onvif channel %d not found: device exposes %d video source(s)", channel, len(sources))
	}

	candidates := bySource[sources[channel-1]]
	for _, profile := range candidates {
		if profile.PTZConfiguration != nil {
			return profile, nil
		}
	}
	return candidates[0], nil
}
