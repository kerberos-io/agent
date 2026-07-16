package onvif

import (
	"bufio"
	"net"
	"strconv"
	"strings"
	"time"
)

// deviceFingerprint holds the identifying information we can gather from a host
// without any credentials. It is populated by grabbing the RTSP and HTTP
// service banners and is then distilled into a manufacturer, model and a
// human-readable device type (e.g. "IP Camera", "DVR/NVR").
type deviceFingerprint struct {
	Manufacturer string
	Model        string
	Type         string
	Server       string
	// realm is the WWW-Authenticate realm advertised by the HTTP service. Many
	// cameras expose their model or vendor here (e.g. realm="Hikvision").
	realm string
	// body holds a lower-cased slice of the HTTP landing page, fetched only when
	// the banners are anonymous. Rebadged/OEM cameras often reveal their vendor
	// there (logo filenames, embedded scripts), e.g. ADI "Capture".
	body string
	// IsCamera is set when the collected evidence confidently identifies the
	// device as a camera, NVR or DVR.
	IsCamera bool
	// IsAudio is set for audio-only devices (IP speakers / intercoms, e.g. TOA)
	// that use RTSP for audio rather than video.
	IsAudio bool
}

// bannerVendors maps a lower-cased substring commonly found in RTSP/HTTP
// service banners or auth realms to a manufacturer. The list is ordered so the
// most specific matches win. This mirrors how tools such as Fing or ONVIF
// Device Manager fingerprint a device from its network banners.
var bannerVendors = []struct {
	Match    string
	Vendor   string
	IsCamera bool
}{
	{"hikvision", "Hikvision", true},
	{"dahua", "Dahua", true},
	{"axis", "Axis", true},
	{"reolink", "Reolink", true},
	{"amcrest", "Amcrest", true},
	{"vivotek", "Vivotek", true},
	{"mobotix", "Mobotix", true},
	{"hanwha", "Hanwha", true},
	{"wisenet", "Hanwha", true},
	{"bosch", "Bosch", true},
	{"foscam", "Foscam", true},
	{"ubiquiti", "Ubiquiti", true},
	{"unifi", "Ubiquiti", true},
	{"uniview", "Uniview", true},
	{"tp-link", "TP-Link", true},
	{"tapo", "TP-Link", true},
	{"linksys", "Linksys", true},
	{"d-link", "D-Link", true},
	{"dlink", "D-Link", true},
	{"trendnet", "Trendnet", true},
	{"lorex", "Lorex", true},
	{"honeywell", "Honeywell", true},
	{"pelco", "Pelco", true},
	{"toa rtsp", "TOA", false},
	{"hipcam", "Hipcam", true},
	{"h264dvr", "Generic DVR", true},
	{"dvrdvs", "Hikvision", true},
	{"webs", "", false}, // generic embedded web server, no vendor
	{"rtsp server", "", true},
	{"gstreamer", "", true},
	{"live555", "", true},
}

// bodyVendors maps a distinctive lower-cased substring found in a camera's HTML
// landing page (logo filename, embedded script, product string) to a
// manufacturer. Used only when the RTSP/HTTP banners are anonymous, so it can
// identify rebadged/OEM cameras (e.g. ADI "Capture") that hide their model
// behind a generic "httpd" server and an "RTSP" realm.
var bodyVendors = []struct {
	Match    string
	Vendor   string
	IsCamera bool
}{
	{"logo_white(capture)", "Capture", true},
	{"logo_capture", "Capture", true},
}

// genericRealms are auth realms that carry no useful model/vendor information.
var genericRealms = map[string]struct{}{
	"":                      {},
	"ip camera":             {},
	"ipcamera":              {},
	"camera":                {},
	"login":                 {},
	"index":                 {},
	"streaming":             {},
	"realm":                 {},
	"network video":         {},
	"web":                   {},
	"protected":             {},
	"authorized users only": {},
	"please log in with a valid username.": {},
	"please log in with a valid username":  {},
}

// fingerprintHost grabs the RTSP and HTTP banners for the given host (based on
// the ports found open during the scan) and classifies the device. It performs
// at most two lightweight, unauthenticated requests and is safe to run
// concurrently for every host.
func fingerprintHost(ip string, openPorts []int, timeout time.Duration) deviceFingerprint {
	var fp deviceFingerprint

	// 1) RTSP OPTIONS on the first open RTSP port. The Server response header of
	//    most camera RTSP stacks reveals the device (e.g. "Dahua Rtsp Server",
	//    "Hipcam RealServer/V1.0", "H264DVR 1.0").
	for _, port := range openPorts {
		if port == 554 || port == 8554 {
			if banner := rtspServerBanner(ip, port, timeout); banner != "" {
				fp.Server = banner
			}
			break
		}
	}

	// 2) HTTP banner + auth realm on the first open HTTP/ONVIF port. Cameras
	//    frequently expose their vendor/model in the Server header or the
	//    WWW-Authenticate realm.
	httpPort := 0
	for _, port := range openPorts {
		if port == 80 || port == 8080 || port == 8000 {
			server, realm := httpBanner(ip, port, timeout)
			if fp.Server == "" {
				fp.Server = server
			}
			fp.realm = realm
			httpPort = port
			break
		}
	}

	// 3) When the banners are anonymous (generic server, no vendor realm), fetch
	//    a slice of the landing page. Rebadged/OEM cameras (e.g. ADI "Capture")
	//    only reveal their vendor in the HTML.
	if httpPort != 0 && isGenericServer(fp.Server) {
		fp.body = httpBody(ip, httpPort, timeout)
	}

	classifyFingerprint(&fp, openPorts)
	return fp
}

// isGenericServer reports whether an HTTP Server header is a generic embedded
// web server that carries no vendor information (so the HTML body is worth a
// look).
func isGenericServer(server string) bool {
	s := strings.ToLower(strings.TrimSpace(server))
	if s == "" {
		return true
	}
	for _, generic := range []string{"httpd", "webs", "boa", "lighttpd", "nginx", "gsoap", "mini_httpd", "thttpd", "apache"} {
		if strings.Contains(s, generic) {
			return true
		}
	}
	return false
}

// httpBody issues an unauthenticated HTTP GET / and returns a lower-cased,
// size-bounded slice of the response (headers + body). Best-effort; empty on
// error.
func httpBody(ip string, port int, timeout time.Duration) string {
	address := net.JoinHostPort(ip, strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return ""
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(timeout))
	request := "GET / HTTP/1.0\r\nHost: " + ip + "\r\nUser-Agent: KerberosDiscovery\r\nAccept: */*\r\n\r\n"
	if _, err := conn.Write([]byte(request)); err != nil {
		return ""
	}
	var builder strings.Builder
	buf := make([]byte, 4096)
	for builder.Len() < 65536 {
		n, err := conn.Read(buf)
		if n > 0 {
			builder.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
	return strings.ToLower(builder.String())
}

// rtspServerBanner issues an unauthenticated RTSP OPTIONS request and returns
// the value of the Server response header (empty when the host does not answer
// or exposes no banner).
func rtspServerBanner(ip string, port int, timeout time.Duration) string {
	address := net.JoinHostPort(ip, strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return ""
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(timeout))
	request := "OPTIONS rtsp://" + address + " RTSP/1.0\r\nCSeq: 1\r\nUser-Agent: KerberosDiscovery\r\n\r\n"
	if _, err := conn.Write([]byte(request)); err != nil {
		return ""
	}
	headers := readBannerHeaders(conn)
	return headers["server"]
}

// httpBanner issues an unauthenticated HTTP HEAD request and returns the Server
// header and the WWW-Authenticate realm (both best-effort, empty when absent).
func httpBanner(ip string, port int, timeout time.Duration) (server string, realm string) {
	address := net.JoinHostPort(ip, strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return "", ""
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(timeout))
	request := "HEAD / HTTP/1.0\r\nHost: " + ip + "\r\nUser-Agent: KerberosDiscovery\r\nAccept: */*\r\n\r\n"
	if _, err := conn.Write([]byte(request)); err != nil {
		return "", ""
	}
	headers := readBannerHeaders(conn)
	return headers["server"], parseRealm(headers["www-authenticate"])
}

// readBannerHeaders reads a status line followed by header lines from an
// RTSP/HTTP response and returns the headers keyed by their lower-cased name.
// Only the first occurrence of a header is kept.
func readBannerHeaders(conn net.Conn) map[string]string {
	headers := make(map[string]string)
	reader := bufio.NewReader(conn)

	// Discard the status line (e.g. "RTSP/1.0 200 OK" or "HTTP/1.1 401 ...").
	if _, err := reader.ReadString('\n'); err != nil {
		return headers
	}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		idx := strings.Index(line, ":")
		if idx <= 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:idx]))
		value := strings.TrimSpace(line[idx+1:])
		if _, exists := headers[key]; !exists {
			headers[key] = value
		}
	}
	return headers
}

// parseRealm extracts the realm token from a WWW-Authenticate header value such
// as `Digest realm="Hikvision", nonce="..."`.
func parseRealm(header string) string {
	lower := strings.ToLower(header)
	marker := "realm="
	idx := strings.Index(lower, marker)
	if idx < 0 {
		return ""
	}
	value := header[idx+len(marker):]
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "\"") {
		value = value[1:]
		if end := strings.Index(value, "\""); end >= 0 {
			value = value[:end]
		}
	} else if end := strings.IndexAny(value, ", "); end >= 0 {
		value = value[:end]
	}
	return strings.TrimSpace(value)
}

// classifyFingerprint distils the collected banners and open ports into a
// manufacturer, model and device type. It also decides whether the evidence is
// strong enough to consider the host a camera/NVR.
func classifyFingerprint(fp *deviceFingerprint, openPorts []int) {
	haystack := strings.ToLower(fp.Server + " " + fp.realm)

	// Manufacturer from the banner/realm.
	for _, entry := range bannerVendors {
		if !strings.Contains(haystack, entry.Match) {
			continue
		}
		if entry.Vendor != "" && fp.Manufacturer == "" {
			fp.Manufacturer = entry.Vendor
		}
		if entry.IsCamera {
			fp.IsCamera = true
		}
		if fp.Manufacturer != "" {
			break
		}
	}

	// Model from the auth realm when it looks specific (not a generic word).
	if fp.Model == "" && fp.realm != "" {
		if _, generic := genericRealms[strings.ToLower(fp.realm)]; !generic {
			if !strings.EqualFold(fp.realm, fp.Manufacturer) {
				fp.Model = fp.realm
			}
		}
	}

	// Vendor from the HTML landing page when the banners revealed nothing.
	// Rebadged/OEM cameras (e.g. ADI "Capture") only identify themselves via
	// logo filenames or embedded scripts.
	if fp.Manufacturer == "" && fp.body != "" {
		for _, entry := range bodyVendors {
			if strings.Contains(fp.body, entry.Match) {
				fp.Manufacturer = entry.Vendor
				if entry.IsCamera {
					fp.IsCamera = true
				}
				break
			}
		}
	}

	// Device type from ports and banners.
	hasRTSP := containsInt(openPorts, 554) || containsInt(openPorts, 8554)
	hasONVIF := containsInt(openPorts, 8000) || containsInt(openPorts, 8899)
	hasDVRPort := containsInt(openPorts, 37777) || containsInt(openPorts, 34567)

	// Audio devices (IP speakers / intercoms) also speak RTSP, but for audio
	// rather than video, so classify them separately and never as a camera.
	if fp.Manufacturer == "TOA" ||
		strings.Contains(haystack, "speaker") ||
		strings.Contains(haystack, "sip audio") ||
		strings.Contains(haystack, "audio server") {
		fp.IsAudio = true
		fp.IsCamera = false
		fp.Type = "IP Speaker/Audio"
		return
	}

	switch {
	case strings.Contains(haystack, "nvr"):
		fp.Type = "NVR"
		fp.IsCamera = true
	case strings.Contains(haystack, "dvr") || hasDVRPort:
		fp.Type = "DVR/NVR"
		fp.IsCamera = true
	case hasRTSP || hasONVIF:
		fp.Type = "IP Camera"
		fp.IsCamera = true
	case fp.IsCamera:
		fp.Type = "IP Camera"
	}
}

func containsInt(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
