package utils

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func RecordingFileName(timestamp int64, deviceKey string) (string, error) {
	if timestamp <= 0 {
		return "", fmt.Errorf("invalid recording timestamp %d", timestamp)
	}
	if deviceKey == "" || strings.ContainsAny(deviceKey, "/\\") {
		return "", fmt.Errorf("missing or invalid recording device key")
	}
	return strconv.FormatInt(timestamp, 10) + "-" + deviceKey + ".mp4", nil
}

func RecordingStartTime(name string) (time.Time, error) {
	base := filepath.Base(name)
	if !strings.HasSuffix(base, ".mp4") {
		return time.Time{}, fmt.Errorf("invalid recording filename %q", name)
	}
	timestamp, deviceKey, short := strings.Cut(strings.TrimSuffix(base, ".mp4"), "-")
	if short && deviceKey != "" {
		if millis, err := strconv.ParseInt(timestamp, 10, 64); err == nil && millis > 0 {
			return time.UnixMilli(millis), nil
		}
	}
	parts := strings.Split(base, "_")
	if len(parts) == 6 {
		seconds, err := strconv.ParseInt(parts[0], 10, 64)
		if err == nil && seconds > 0 {
			return time.Unix(seconds, 0), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid recording filename %q", name)
}

func CreateRecordingPath(directory, fileName string, recordedAt time.Time) (string, error) {
	if !filepath.IsLocal(fileName) || fileName == "." || filepath.Base(fileName) != fileName || strings.ContainsAny(fileName, "/\\") {
		return "", fmt.Errorf("invalid recording filename %q", fileName)
	}
	if recordedAt.UnixMilli() <= 0 {
		return "", fmt.Errorf("invalid recording start time %s", recordedAt)
	}
	directory = filepath.Join(directory, recordedAt.UTC().Format("2006/01/02"))
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", fmt.Errorf("create recording directory: %w", err)
	}
	return filepath.Join(directory, fileName), nil
}

// ResolveRecordingPath accepts relative paths from listings and historical
// basename-only references from upload markers and playback URLs.
func ResolveRecordingPath(directory, name string) (string, error) {
	if !filepath.IsLocal(name) {
		return "", fmt.Errorf("invalid recording path %q", name)
	}
	filePath := filepath.Join(directory, name)
	info, err := os.Stat(filePath)
	if os.IsNotExist(err) && filepath.Base(name) == name {
		recordedAt, parseErr := RecordingStartTime(name)
		if parseErr == nil {
			filePath = filepath.Join(directory, recordedAt.UTC().Format("2006/01/02"), name)
			info, err = os.Stat(filePath)
		}
	}
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("recording %q is not a regular file", name)
	}
	return filePath, nil
}

func OpenRecording(directory, name string) (*os.File, error) {
	filePath, err := ResolveRecordingPath(directory, name)
	if err != nil {
		return nil, err
	}
	return os.Open(filePath)
}

func RemoveRecording(directory, name string) error {
	filePath, err := ResolveRecordingPath(directory, name)
	if err != nil {
		return err
	}
	return os.Remove(filePath)
}

type recordingFileInfo struct {
	os.FileInfo
	relativePath string
}

func (file recordingFileInfo) Name() string {
	return file.relativePath
}

// ReadRecordingDirectory returns MP4s with root-relative names, preserving the
// file information used by the existing sorting and media formatting helpers.
func ReadRecordingDirectory(directory string) ([]os.FileInfo, error) {
	return readRecordingDirectory(directory, filepath.WalkDir)
}

func readRecordingDirectory(directory string, walkDir func(string, fs.WalkDirFunc) error) ([]os.FileInfo, error) {
	files := []os.FileInfo{}
	err := walkDir(directory, func(filePath string, entry fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) && filePath != directory {
				return nil
			}
			return err
		}
		if entry.IsDir() && entry.Name() == "decrypted" && filePath != directory {
			return filepath.SkipDir
		}
		if !entry.Type().IsRegular() || filepath.Ext(entry.Name()) != ".mp4" {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		name, err := filepath.Rel(directory, filePath)
		if err != nil {
			return err
		}
		files = append(files, recordingFileInfo{FileInfo: info, relativePath: filepath.ToSlash(name)})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}
