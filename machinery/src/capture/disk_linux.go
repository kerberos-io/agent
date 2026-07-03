//go:build linux

package capture

import "syscall"

// diskUsageMB returns the total capacity and the currently available space (both
// in megabytes, decimal) of the filesystem that contains path. Auto-clean uses
// it to default its cleanup threshold to the real disk capacity instead of a
// fixed size, so recordings can grow to fill the disk while keeping a reserve
// free. Linux is the agent's deployment target (amd64/arm64 containers).
func diskUsageMB(path string) (totalMB int64, availableMB int64, err error) {
	var stat syscall.Statfs_t
	if err = syscall.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}
	blockSize := int64(stat.Bsize)
	totalMB = int64(stat.Blocks) * blockSize / 1000 / 1000
	// Bavail is the free space available to unprivileged users, which is the
	// space we can actually keep writing recordings into.
	availableMB = int64(stat.Bavail) * blockSize / 1000 / 1000
	return totalMB, availableMB, nil
}
