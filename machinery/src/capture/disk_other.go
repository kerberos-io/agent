//go:build !linux

package capture

import "errors"

// diskUsageMB is only implemented on Linux (the agent's deployment target). On
// other platforms (e.g. local macOS/Windows dev builds) auto-clean falls back to
// its historical fixed-size directory cap, so this reports the capability as
// unavailable.
func diskUsageMB(path string) (totalMB int64, availableMB int64, err error) {
	return 0, 0, errors.New("disk usage stats are not supported on this platform")
}
