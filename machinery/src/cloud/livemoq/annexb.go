package livemoq

import (
	"bytes"
	"strings"

	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
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
