package onvif

import (
	"bufio"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/kerberos-io/agent/machinery/src/models"
)

// brandProfile describes a camera brand together with the RTSP URL path
// templates it exposes for its main (high quality) and sub (low quality)
// streams. The paths are the well-known, widely documented defaults for each
// vendor and are used both to identify the brand (by probing which path the
// device recognises) and to pre-fill a working RTSP URL for the user.
//
// The order of the list matters: more specific / more common brands come first
// so that when we actively probe a device the first matching profile wins.
type brandProfile struct {
	Brand string
	// aliases are lower-cased tokens that, when seen in a banner/realm/MAC
	// vendor, map onto this brand.
	Aliases  []string
	MainPath string
	SubPath  string
	// extraMainPaths are alternative main-stream paths tried during active
	// probing when the primary MainPath is not recognised.
	extraMainPaths []string
}

// brandProfiles is the built-in brand -> RTSP path mapping. It mirrors the
// tables used by tools such as ONVIF Device Manager, iSpy/Agent DVR and
// Blue Iris.
var brandProfiles = []brandProfile{
	{
		Brand:          "Hikvision",
		Aliases:        []string{"hikvision", "dvrdvs", "ds-", "hik"},
		MainPath:       "/Streaming/Channels/101",
		SubPath:        "/Streaming/Channels/102",
		extraMainPaths: []string{"/h264/ch1/main/av_stream", "/ISAPI/Streaming/Channels/101"},
	},
	{
		Brand:          "Dahua",
		Aliases:        []string{"dahua", "dh-"},
		MainPath:       "/cam/realmonitor?channel=1&subtype=0",
		SubPath:        "/cam/realmonitor?channel=1&subtype=1",
		extraMainPaths: []string{"/live"},
	},
	{
		Brand:    "Amcrest",
		Aliases:  []string{"amcrest"},
		MainPath: "/cam/realmonitor?channel=1&subtype=0",
		SubPath:  "/cam/realmonitor?channel=1&subtype=1",
	},
	{
		Brand:          "Axis",
		Aliases:        []string{"axis"},
		MainPath:       "/axis-media/media.amp",
		SubPath:        "/axis-media/media.amp?videocodec=h264&resolution=640x480",
		extraMainPaths: []string{"/mpeg4/media.amp"},
	},
	{
		Brand:          "Reolink",
		Aliases:        []string{"reolink", "rlc", "rln", "rlc-", "rln-", "trackmix", "duo"},
		MainPath:       "/h264Preview_01_main",
		SubPath:        "/h264Preview_01_sub",
		extraMainPaths: []string{"/Preview_01_main"},
	},
	{
		Brand:          "Hanwha",
		Aliases:        []string{"hanwha", "wisenet", "samsung techwin"},
		MainPath:       "/profile2/media.smp",
		SubPath:        "/profile3/media.smp",
		extraMainPaths: []string{"/profile1/media.smp", "/onvif/profile2/media.smp"},
	},
	{
		Brand:          "Bosch",
		Aliases:        []string{"bosch"},
		MainPath:       "/rtsp_tunnel",
		SubPath:        "/rtsp_tunnel?inst=2",
		extraMainPaths: []string{"/rtsp_tunnel?inst=1", "/?inst=1"},
	},
	{
		Brand:          "Vivotek",
		Aliases:        []string{"vivotek"},
		MainPath:       "/live.sdp",
		SubPath:        "/live2.sdp",
		extraMainPaths: []string{"/live1s1.sdp"},
	},
	{
		Brand:    "Foscam",
		Aliases:  []string{"foscam"},
		MainPath: "/videoMain",
		SubPath:  "/videoSub",
	},
	{
		Brand:          "Uniview",
		Aliases:        []string{"uniview", "unv"},
		MainPath:       "/media/video1",
		SubPath:        "/media/video2",
		extraMainPaths: []string{"/unicast/c1/s0/live", "/unicast/c1/s1/live"},
	},
	{
		Brand:    "TP-Link",
		Aliases:  []string{"tp-link", "tplink", "tapo"},
		MainPath: "/stream1",
		SubPath:  "/stream2",
	},
	{
		Brand:          "Mobotix",
		Aliases:        []string{"mobotix"},
		MainPath:       "/cam0/mjpeg",
		SubPath:        "/cam1/mjpeg",
		extraMainPaths: []string{"/live.sdp"},
	},
	{
		Brand:          "Ubiquiti",
		Aliases:        []string{"ubiquiti", "unifi"},
		MainPath:       "/s0",
		SubPath:        "/s1",
		extraMainPaths: []string{"/live/ch00_0"},
	},
	{
		Brand:    "Panasonic",
		Aliases:  []string{"panasonic", "i-pro", "ipro"},
		MainPath: "/MediaInput/h264",
		SubPath:  "/MediaInput/h264/stream_2",
	},
	{
		Brand:    "Sony",
		Aliases:  []string{"sony"},
		MainPath: "/media/video1",
		SubPath:  "/media/video2",
	},
	{
		// D-Link mydlink IP cameras. Older models stream MJPEG over HTTP; the
		// RTSP-capable ones expose SDP-named streams, newer DCS models use
		// "/live/profile.0".
		Brand:          "D-Link",
		Aliases:        []string{"d-link", "dlink", "dcs-", "dcs"},
		MainPath:       "/live1.sdp",
		SubPath:        "/live2.sdp",
		extraMainPaths: []string{"/live.sdp", "/live/profile.0", "/play1.sdp"},
	},
	{
		// TRENDnet. Newer PoE bullet/dome models (TV-IPxxxPI) use a
		// Hikvision-style path; older ones expose SDP streams.
		Brand:          "Trendnet",
		Aliases:        []string{"trendnet", "tv-ip"},
		MainPath:       "/Streaming/Channels/101",
		SubPath:        "/Streaming/Channels/102",
		extraMainPaths: []string{"/play1.sdp", "/play2.sdp", "/ch0_0.h264", "/live/av0"},
	},
	{
		// Lorex is built largely on Dahua hardware, so it shares Dahua's
		// realmonitor path scheme.
		Brand:          "Lorex",
		Aliases:        []string{"lorex"},
		MainPath:       "/cam/realmonitor?channel=1&subtype=0",
		SubPath:        "/cam/realmonitor?channel=1&subtype=1",
		extraMainPaths: []string{"/ch01/0"},
	},
	{
		// Honeywell ships both Dahua-OEM models (realmonitor) and in-house
		// firmwares exposing "/h264" or "/media".
		Brand:          "Honeywell",
		Aliases:        []string{"honeywell"},
		MainPath:       "/cam/realmonitor?channel=1&subtype=0",
		SubPath:        "/cam/realmonitor?channel=1&subtype=1",
		extraMainPaths: []string{"/h264", "/media", "/live.sdp"},
	},
	{
		Brand:          "Pelco",
		Aliases:        []string{"pelco"},
		MainPath:       "/stream1",
		SubPath:        "/stream2",
		extraMainPaths: []string{"/1/stream1"},
	},
	{
		// TOA network audio devices (IP horn speakers / intercoms, banner
		// "TOA rtsp server") expose their stream through ONVIF rather than a
		// documented fixed RTSP path. These ONVIF-style paths are a best-effort
		// default; the authoritative URL should come from an ONVIF GetStreamUri
		// query with credentials.
		Brand:          "TOA",
		Aliases:        []string{"toa"},
		MainPath:       "/ONVIF/channel1",
		SubPath:        "/ONVIF/channel2",
		extraMainPaths: []string{"/media/video1", "/live"},
	},
	{
		// Linksys/Cisco IP cameras (e.g. LCAD03FLN, LCAB03VLNOD, LCAM0336OD)
		// run a mini_httpd server and expose ONVIF-style stream paths with a
		// capitalised "ONVIF" segment (distinct from the generic "/onvif1").
		Brand:          "Linksys",
		Aliases:        []string{"linksys", "lcad", "lcab", "lcam", "lcae"},
		MainPath:       "/ONVIF/channel1",
		SubPath:        "/ONVIF/channel2",
		extraMainPaths: []string{"/img/media.sav", "/live"},
	},
}

// genericRTSPPaths are last-resort, vendor-neutral RTSP paths used when the
// brand is unknown. Many ONVIF/embedded cameras answer on one of these.
var genericRTSPPaths = []string{
	"/ONVIF/channel1", "/ONVIF/channel2", "/onvif1", "/live", "/live/ch0", "/11", "/12",
	"/stream0", "/stream1", "/h264", "/media/video1", "/ch0_0.h264",
}

// brandProfileFor returns the profile whose aliases best match the given brand
// hint (from a banner, realm or MAC vendor). It returns nil when nothing
// matches.
func brandProfileFor(hint string) *brandProfile {
	hint = strings.ToLower(strings.TrimSpace(hint))
	if hint == "" {
		return nil
	}
	for i := range brandProfiles {
		for _, alias := range brandProfiles[i].Aliases {
			if strings.Contains(hint, alias) {
				return &brandProfiles[i]
			}
		}
	}
	return nil
}

// realmBrands maps a lower-cased substring of an RTSP/HTTP WWW-Authenticate
// realm to a manufacturer. The auth realm is one of the most reliable brand
// signals because a camera advertises it even when it refuses every
// unauthenticated request (e.g. Hikvision realm "IP Camera(E3669)", Dahua realm
// "Login to <serial>"). Ordered so the most specific matches win.
var realmBrands = []struct {
	Match  string
	Vendor string
}{
	{"login to", "Dahua"},
	{"surveillance server", "Dahua"},
	{"real time streaming", "Dahua"},
	{"dahua", "Dahua"},
	{"ip camera(", "Hikvision"},
	{"hikvision", "Hikvision"},
	{"ds-", "Hikvision"},
	{"axis", "Axis"},
	{"reolink", "Reolink"},
	{"amcrest", "Amcrest"},
	{"wisenet", "Hanwha"},
	{"hanwha", "Hanwha"},
	{"uniview", "Uniview"},
	{"tp-link", "TP-Link"},
	{"tapo", "TP-Link"},
	{"foscam", "Foscam"},
	{"vivotek", "Vivotek"},
	{"mobotix", "Mobotix"},
	{"bosch", "Bosch"},
	{"please log in with a valid username", "Bosch"},
	{"d-link", "D-Link"},
	{"dcs-", "D-Link"},
	{"trendnet", "Trendnet"},
	{"lorex", "Lorex"},
	{"honeywell", "Honeywell"},
	{"pelco", "Pelco"},
	{"linksys", "Linksys"},
	{"lcad", "Linksys"},
	{"lcab", "Linksys"},
	{"lcam", "Linksys"},
}

// brandFromRealm resolves a manufacturer from an auth realm string.
func brandFromRealm(realm string) string {
	r := strings.ToLower(strings.TrimSpace(realm))
	if r == "" {
		return ""
	}
	for _, entry := range realmBrands {
		if strings.Contains(r, entry.Match) {
			return entry.Vendor
		}
	}
	return ""
}

// modelFromRealm extracts a model/device code embedded in an auth realm, e.g.
// Hikvision's realm="IP Camera(E3669)" -> "E3669".
func modelFromRealm(realm string) string {
	open := strings.Index(realm, "(")
	closeIdx := strings.Index(realm, ")")
	if open >= 0 && closeIdx > open+1 {
		return strings.TrimSpace(realm[open+1 : closeIdx])
	}
	return ""
}

// firstNonEmpty returns the first non-blank value.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// guessRTSPStreams determines the most likely RTSP stream URLs for a camera. It
// combines the brand hint discovered from banners/MAC with an active,
// unauthenticated RTSP DESCRIBE probe and the auth realm advertised by the
// device.
//
// Detection strategy (most reliable first):
//  1. Send a control DESCRIBE for a random, non-existent path. Its 401 response
//     usually carries a WWW-Authenticate realm that reveals the brand
//     (Hikvision "IP Camera(...)", Dahua "Login to ..."). The realm is the
//     strongest signal and works even when the device challenges auth for every
//     request. The control also tells us whether the device distinguishes valid
//     from invalid paths.
//  2. If the device discriminates paths, probe each brand's main path (realm
//     brand first); the first the device recognises (200 or 401/403) confirms a
//     working URL.
//  3. Otherwise fall back to the realm / hint / port brand's default paths and
//     return them as unverified suggestions.
//
// It returns the detected brand, an optional model code parsed from the realm,
// and the ordered list of candidate streams (verified first).
func guessRTSPStreams(ip string, port int, brandHint string, openPorts []int, timeout time.Duration) (brand string, model string, streams []models.RTSPStream) {
	base := "rtsp://" + net.JoinHostPort(ip, strconv.Itoa(port))

	build := func(profileBrand, stream, path string, verified, requiresAuth bool) models.RTSPStream {
		return models.RTSPStream{
			Brand:        profileBrand,
			Stream:       stream,
			Path:         path,
			URL:          base + path,
			Verified:     verified,
			RequiresAuth: requiresAuth,
		}
	}

	// 1) Control probe: distinguish behaviour + capture the auth realm.
	bogusPath := "/kerberos-probe-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	controlStatus, controlRealm, _ := rtspDescribe(ip, port, bogusPath, timeout)
	controlExists := controlStatus == 200 || controlStatus == 401 || controlStatus == 403
	controlAuth := controlStatus == 401 || controlStatus == 403
	discriminates := !controlExists

	realmBrand := brandFromRealm(controlRealm)
	model = modelFromRealm(controlRealm)

	// The realm brand (when present) is authoritative and probed first.
	primaryHint := firstNonEmpty(realmBrand, brandHint)

	var verified []models.RTSPStream
	var unverified []models.RTSPStream
	detected := ""

	// 2) Trustworthy active per-brand probing (device discriminates paths).
	if discriminates {
		for _, profile := range orderedProfiles(primaryHint) {
			mainCandidates := append([]string{profile.MainPath}, profile.extraMainPaths...)
			matchedMain := ""
			matchedAuth := false
			for _, path := range mainCandidates {
				ok, requiresAuth := rtspPathExists(ip, port, path, timeout)
				if ok {
					matchedMain = path
					matchedAuth = requiresAuth
					break
				}
			}
			if matchedMain == "" {
				continue
			}
			detected = profile.Brand
			verified = append(verified, build(profile.Brand, "main", matchedMain, true, matchedAuth))
			if profile.SubPath != "" {
				subOK, subAuth := rtspPathExists(ip, port, profile.SubPath, timeout)
				verified = append(verified, build(profile.Brand, "sub", profile.SubPath, subOK, subAuth || matchedAuth))
			}
			break
		}
	}

	// 3) Fall back to unverified suggestions from realm / hint / port signals.
	if len(verified) == 0 {
		profile := brandProfileFor(primaryHint)
		if profile == nil {
			profile = brandProfileForPorts(openPorts)
		}
		if profile != nil {
			detected = profile.Brand
			unverified = append(unverified, build(profile.Brand, "main", profile.MainPath, false, controlAuth))
			if profile.SubPath != "" {
				unverified = append(unverified, build(profile.Brand, "sub", profile.SubPath, false, controlAuth))
			}
		} else {
			for _, path := range genericRTSPPaths {
				unverified = append(unverified, build("Generic", "main", path, false, controlAuth))
			}
		}
	}

	// The realm brand always wins for the manufacturer name.
	if realmBrand != "" {
		detected = realmBrand
	}

	return detected, model, append(verified, unverified...)
}

// brandProfileForPorts derives a brand from vendor-specific control ports that
// were found open during the scan (used when banners give no hint).
func brandProfileForPorts(openPorts []int) *brandProfile {
	if containsInt(openPorts, 37777) {
		return brandProfileByName("Dahua")
	}
	return nil
}

// brandProfileByName returns the profile with the given brand name (nil when
// absent).
func brandProfileByName(name string) *brandProfile {
	for i := range brandProfiles {
		if brandProfiles[i].Brand == name {
			return &brandProfiles[i]
		}
	}
	return nil
}

// orderedProfiles returns the brand profiles with the profile matching the
// brand hint (if any) moved to the front so it is probed first.
func orderedProfiles(brandHint string) []brandProfile {
	match := brandProfileFor(brandHint)
	if match == nil {
		return brandProfiles
	}
	ordered := make([]brandProfile, 0, len(brandProfiles))
	ordered = append(ordered, *match)
	for i := range brandProfiles {
		if brandProfiles[i].Brand != match.Brand {
			ordered = append(ordered, brandProfiles[i])
		}
	}
	return ordered
}

// rtspDescribe sends an unauthenticated RTSP DESCRIBE for the given path and
// returns the response status code together with the WWW-Authenticate realm and
// Server header (when present). status is 0 when the device does not answer.
func rtspDescribe(ip string, port int, path string, timeout time.Duration) (status int, realm string, server string) {
	address := net.JoinHostPort(ip, strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return 0, "", ""
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(timeout))
	request := "DESCRIBE rtsp://" + address + path + " RTSP/1.0\r\n" +
		"CSeq: 1\r\n" +
		"User-Agent: KerberosDiscovery\r\n" +
		"Accept: application/sdp\r\n\r\n"
	if _, err := conn.Write([]byte(request)); err != nil {
		return 0, "", ""
	}

	status, headers := readRTSPResponse(conn)
	return status, parseRealm(headers["www-authenticate"]), headers["server"]
}

// rtspPathExists reports whether the device recognises the given RTSP path. A
// 200 OK means the path is publicly accessible; a 401/403 means the path is
// valid but requires credentials (still a positive match). Any other status
// (404, 400, 455, ...) means the path is not recognised.
func rtspPathExists(ip string, port int, path string, timeout time.Duration) (exists bool, requiresAuth bool) {
	status, _, _ := rtspDescribe(ip, port, path, timeout)
	switch status {
	case 200:
		return true, false
	case 401, 403:
		return true, true
	default:
		return false, false
	}
}

// readRTSPResponse reads and parses the status code and headers of an RTSP
// response. Only the first occurrence of each header is kept.
func readRTSPResponse(conn net.Conn) (status int, headers map[string]string) {
	headers = make(map[string]string)
	reader := bufio.NewReader(conn)

	line, err := reader.ReadString('\n')
	if err != nil {
		return 0, headers
	}
	fields := strings.Fields(line)
	if len(fields) >= 2 && strings.HasPrefix(strings.ToUpper(fields[0]), "RTSP/") {
		status, _ = strconv.Atoi(fields[1])
	}

	for {
		hline, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		hline = strings.TrimRight(hline, "\r\n")
		if hline == "" {
			break
		}
		idx := strings.Index(hline, ":")
		if idx <= 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(hline[:idx]))
		value := strings.TrimSpace(hline[idx+1:])
		if _, exists := headers[key]; !exists {
			headers[key] = value
		}
	}
	return status, headers
}
