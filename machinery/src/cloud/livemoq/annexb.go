package livemoq

import "strings"

var annexBStartCode = []byte{0x00, 0x00, 0x00, 0x01}

// EnsureAnnexB restores the start code stripped by the Agent capture queue.
func EnsureAnnexB(payload []byte) []byte {
	if hasAnnexBStartCode(payload) {
		return payload
	}

	framed := make([]byte, 0, len(annexBStartCode)+len(payload))
	framed = append(framed, annexBStartCode...)
	return append(framed, payload...)
}

func BroadcastPath(prefix string, deviceKey string) string {
	prefix = strings.Trim(prefix, "/")
	if prefix == "" {
		prefix = "devices"
	}
	return prefix + "/" + strings.Trim(deviceKey, "/") + "/live.hang"
}

func hasAnnexBStartCode(payload []byte) bool {
	return len(payload) >= 4 && payload[0] == 0 && payload[1] == 0 &&
		((payload[2] == 0 && payload[3] == 1) || payload[2] == 1)
}
