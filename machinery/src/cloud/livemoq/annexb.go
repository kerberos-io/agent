package livemoq

import (
	"bytes"
	"strings"

	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/kerberos-io/agent/machinery/src/models"
)

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

// NormalizeH264AccessUnit removes delimiters and duplicate parameter sets that
// can make older MoQ splitters emit a parameter-only frame before the IDR.
func NormalizeH264AccessUnit(payload []byte) ([]byte, error) {
	nalus, err := h264.AnnexBUnmarshal(EnsureAnnexB(payload))
	if err != nil {
		return nil, err
	}

	normalized := make([][]byte, 0, len(nalus))
	for _, nalu := range nalus {
		if len(nalu) == 0 || nalu[0]&0x1f == 9 {
			continue
		}
		if nalu[0]&0x1f == 7 || nalu[0]&0x1f == 8 {
			duplicate := false
			for _, existing := range normalized {
				if bytes.Equal(existing, nalu) {
					duplicate = true
					break
				}
			}
			if duplicate {
				continue
			}
		}
		normalized = append(normalized, nalu)
	}

	return h264.AnnexBMarshal(normalized)
}

// BroadcastPath returns the relay path a quality tier is published on. Every
// tier gets its own broadcast so a viewer switches between the camera's main and
// sub stream by resubscribing to another path, without any control channel back
// to the Agent. The high tier keeps the historical ".../live.hang" path so
// existing viewers keep working; the low tier lives next to it on
// ".../live-low.hang".
func BroadcastPath(prefix string, deviceKey string, quality string) string {
	prefix = strings.Trim(prefix, "/")
	if prefix == "" {
		prefix = "devices"
	}
	name := "live.hang"
	if quality == models.StreamQualityLow {
		name = "live-low.hang"
	}
	return prefix + "/" + strings.Trim(deviceKey, "/") + "/" + name
}

// TimestampUs converts the capture presentation timestamp from milliseconds.
// CompositionTime must not be added: it is already represented in the PTS and
// is only used by muxers to derive DTS for streams containing B-frames.
func TimestampUs(presentationTimeMs int64) uint64 {
	if presentationTimeMs < 0 {
		return 0
	}
	return uint64(presentationTimeMs) * 1000
}

func hasAnnexBStartCode(payload []byte) bool {
	return len(payload) >= 4 && payload[0] == 0 && payload[1] == 0 &&
		((payload[2] == 0 && payload[3] == 1) || payload[2] == 1)
}
