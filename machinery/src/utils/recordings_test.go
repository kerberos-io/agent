package utils

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/kerberos-io/agent/machinery/src/encryption"
	"github.com/kerberos-io/agent/machinery/src/models"
)

func TestShortRecordingNamePreservesMillisecondsAndDeviceKey(t *testing.T) {
	const timestamp = int64(1767355200123)
	const deviceKey = "device-key_with-hyphens"
	name, err := RecordingFileName(timestamp, deviceKey)
	if err != nil || name != "1767355200123-device-key_with-hyphens.mp4" {
		t.Fatalf("RecordingFileName() = %q, %v", name, err)
	}
	other, err := RecordingFileName(timestamp+1, deviceKey)
	if err != nil || other == name {
		t.Fatalf("recordings within the same second must have distinct names: %q, %v", other, err)
	}
	directory := t.TempDir()
	filePath, err := CreateRecordingPath(directory, name, time.UnixMilli(timestamp))
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(directory, "2026/01/02", name); filePath != want {
		t.Fatalf("path = %q, want %q", filePath, want)
	}
	if err := os.WriteFile(filePath, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolved, err := ResolveRecordingPath(directory, name)
	if err != nil || resolved != filePath {
		t.Fatalf("ResolveRecordingPath() = %q, %v", resolved, err)
	}
	files, err := ReadRecordingDirectory(directory)
	if err != nil {
		t.Fatal(err)
	}
	config := &models.Configuration{Config: models.Config{Timezone: "UTC"}}
	media := GetMediaFormatted(files, directory, config, models.EventFilter{TimestampOffsetStart: timestamp / 1000})
	if len(media) != 1 || media[0].Key != "2026/01/02/"+name || media[0].Timestamp != "1767355200" {
		t.Fatalf("unexpected short-name media: %+v", media)
	}
	if days := GetDays(files, directory, config); !reflect.DeepEqual(days, []string{"02-01-2026"}) {
		t.Fatalf("unexpected short-name days: %v", days)
	}
	if err := RemoveRecording(directory, name); err != nil {
		t.Fatal(err)
	}
	for _, invalidKey := range []string{"", "camera/key", `camera\key`} {
		if _, err := RecordingFileName(timestamp, invalidKey); err == nil {
			t.Errorf("accepted invalid device key %q", invalidKey)
		}
	}
	if _, err := RecordingFileName(0, deviceKey); err == nil {
		t.Fatal("accepted invalid recording timestamp")
	}
}

func TestCreateRecordingPathUsesRecordingUTCDate(t *testing.T) {
	directory := t.TempDir()
	recordedAt := time.Date(2026, time.January, 1, 1, 0, 0, 0, time.FixedZone("UTC+2", 2*60*60))
	name := strconv.FormatInt(recordedAt.Unix(), 10) + "_3-123_camera_0-0-0-0_1_1000.mp4"
	filePath, err := CreateRecordingPath(directory, name, recordedAt)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(directory, "2025/12/31", name); filePath != want {
		t.Fatalf("path = %q, want %q", filePath, want)
	}
	if err := os.WriteFile(filePath, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, reference := range []string{name, "2025/12/31/" + name} {
		got, err := ResolveRecordingPath(directory, reference)
		if err != nil || got != filePath {
			t.Fatalf("ResolveRecordingPath(%q) = %q, %v; want %q", reference, got, err, filePath)
		}
		file, err := OpenRecording(directory, reference)
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(file)
		file.Close()
		if err != nil || string(content) != "video" {
			t.Fatalf("recording content = %q, %v", content, err)
		}
	}
	if err := RemoveRecording(directory, name); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatalf("recording still exists after removal: %v", err)
	}
}

func TestRecordingPathsKeepFlatFilesAccessible(t *testing.T) {
	directory := t.TempDir()
	name := "legacy-recording.mp4"
	filePath := filepath.Join(directory, name)
	if err := os.WriteFile(filePath, []byte("legacy"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveRecordingPath(directory, name)
	if err != nil || got != filePath {
		t.Fatalf("ResolveRecordingPath() = %q, %v; want %q", got, err, filePath)
	}
	if err := RemoveRecording(directory, name); err != nil {
		t.Fatal(err)
	}
}

func TestRecordingPathsReportInvalidAndMissingPaths(t *testing.T) {
	directory := t.TempDir()
	for _, name := range []string{"", ".", "..", "../outside.mp4", "/outside.mp4", "nested/recording.mp4", `nested\recording.mp4`} {
		if _, err := CreateRecordingPath(directory, name, time.Now()); err == nil {
			t.Errorf("CreateRecordingPath(%q) should reject invalid filename", name)
		}
	}
	for _, name := range []string{"", ".", "../outside.mp4", "/outside.mp4"} {
		if _, err := ResolveRecordingPath(directory, name); err == nil {
			t.Errorf("ResolveRecordingPath(%q) should reject invalid path", name)
		}
	}
	for _, name := range []string{"missing.mp4", "1700000000_missing.mp4", "2026/01/01/missing.mp4"} {
		if _, err := ResolveRecordingPath(directory, name); !os.IsNotExist(err) {
			t.Errorf("ResolveRecordingPath(%q) error = %v, want not-exist", name, err)
		}
	}
	blocked := filepath.Join(directory, "not-a-directory")
	if err := os.WriteFile(blocked, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateRecordingPath(blocked, "recording.mp4", time.Now()); err == nil {
		t.Fatal("expected directory creation error")
	}
	if _, err := CreateRecordingPath(directory, "recording.mp4", time.Time{}); err == nil {
		t.Fatal("expected invalid recording start time error")
	}
	if _, err := ReadRecordingDirectory(filepath.Join(directory, "missing")); !os.IsNotExist(err) {
		t.Fatalf("ReadRecordingDirectory() error = %v, want not-exist", err)
	}
}

func TestReadRecordingDirectoryIncludesFlatAndDatedMedia(t *testing.T) {
	directory := t.TempDir()
	names := []string{
		"1700000000_6_7_8_9_10.mp4",
		"2026/01/02/1767355200_6_7_8_9_10.mp4",
	}
	for index, name := range names {
		filePath := filepath.Join(directory, name)
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filePath, []byte("video"), 0o644); err != nil {
			t.Fatal(err)
		}
		modified := time.Unix(int64(index+1), 0)
		if err := os.Chtimes(filePath, modified, modified); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(directory, "2026/01/02/ignore.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(directory, "2026/01/02/decrypted"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "2026/01/02/decrypted/output.mp4"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := ReadRecordingDirectory(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || NumberOfMP4sInDirectory(directory) != 2 {
		t.Fatalf("expected 2 MP4 recordings, got %d", len(files))
	}
	config := &models.Configuration{Config: models.Config{Timezone: "UTC", Name: "camera", Key: "device"}}
	files = GetSortedDirectory(files)
	media := GetMediaFormatted(files, directory, config, models.EventFilter{})
	if len(media) != 2 || media[0].Key != names[1] || media[1].Key != names[0] {
		t.Fatalf("unexpected media ordering or relative keys: %+v", media)
	}
	if media[0].Path != filepath.Join(directory, names[1]) || media[0].Timestamp != "1767355200" {
		t.Fatalf("unexpected nested media path or timestamp: %+v", media[0])
	}
	if got, want := GetDays(files, directory, config), []string{"02-01-2026", "14-11-2023"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("days = %v, want %v", got, want)
	}
	filtered := GetMediaFormatted(files, directory, config, models.EventFilter{TimestampOffsetStart: 1767355200})
	if len(filtered) != 1 || filtered[0].Key != names[1] {
		t.Fatalf("unexpected filtered nested media: %+v", filtered)
	}
}

func TestReadRecordingDirectoryContinuesAfterConcurrentDeletion(t *testing.T) {
	for _, origin := range []string{"Info", "WalkDir"} {
		t.Run(origin, func(t *testing.T) {
			directory := t.TempDir()
			for _, name := range []string{"deleted.mp4", "surviving.mp4"} {
				if err := os.WriteFile(filepath.Join(directory, name), []byte("video"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			entries, err := os.ReadDir(directory)
			if err != nil {
				t.Fatal(err)
			}
			files, err := readRecordingDirectory(directory, func(root string, visit fs.WalkDirFunc) error {
				deleted := filepath.Join(root, entries[0].Name())
				if err := os.Remove(deleted); err != nil {
					t.Fatal(err)
				}
				var err error
				if origin == "Info" {
					err = visit(deleted, entries[0], nil)
				} else {
					err = visit(deleted, nil, &fs.PathError{Op: "lstat", Path: deleted, Err: fs.ErrNotExist})
				}
				if err != nil {
					return err
				}
				return visit(filepath.Join(root, entries[1].Name()), entries[1], nil)
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(files) != 1 || files[0].Name() != "surviving.mp4" || files[0].Size() != 5 {
				t.Fatalf("expected surviving recording after concurrent deletion, got %v", files)
			}
		})
	}
}

type recordingDirEntryInfoError struct {
	fs.DirEntry
	err error
}

func (entry recordingDirEntryInfoError) Info() (fs.FileInfo, error) {
	return nil, entry.err
}

func TestReadRecordingDirectoryReportsOtherScanErrors(t *testing.T) {
	for _, origin := range []string{"Info", "WalkDir"} {
		t.Run(origin, func(t *testing.T) {
			directory := t.TempDir()
			filePath := filepath.Join(directory, "recording.mp4")
			if err := os.WriteFile(filePath, []byte("video"), 0o644); err != nil {
				t.Fatal(err)
			}
			entries, err := os.ReadDir(directory)
			if err != nil {
				t.Fatal(err)
			}
			wantErr := &fs.PathError{Op: "lstat", Path: filePath, Err: fs.ErrPermission}
			files, err := readRecordingDirectory(directory, func(root string, visit fs.WalkDirFunc) error {
				if origin == "Info" {
					return visit(filePath, recordingDirEntryInfoError{DirEntry: entries[0], err: wantErr}, nil)
				}
				return visit(filePath, nil, wantErr)
			})
			if !errors.Is(err, fs.ErrPermission) || files != nil {
				t.Fatalf("scan = %v, %v; want permission error without partial results", files, err)
			}
		})
	}
}

func TestDecryptDirectoryIncludesDatedRecordingsAndSkipsOutputs(t *testing.T) {
	directory := t.TempDir()
	key := []byte("0123456789abcdef0123456789abcdef")
	payload := []byte("recording-payload")
	ciphertext, err := encryption.AesEncrypt(payload, string(key))
	if err != nil {
		t.Fatal(err)
	}
	names := []string{"legacy.mp4", "2026/01/02/recording.mp4"}
	for _, name := range names {
		filePath := filepath.Join(directory, name)
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filePath, []byte(ciphertext), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	Decrypt(directory, key)
	Decrypt(directory, key)
	for _, name := range names {
		output := filepath.Join(directory, filepath.Dir(name), "decrypted", filepath.Base(name))
		content, err := os.ReadFile(output)
		if err != nil || string(content) != string(payload) {
			t.Fatalf("decrypted %q = %q, %v", name, content, err)
		}
	}
	if got := NumberOfMP4sInDirectory(directory); got != len(names) {
		t.Fatalf("recording count = %d, want %d (excluding decrypted outputs)", got, len(names))
	}
}
