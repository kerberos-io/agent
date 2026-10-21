package cloud

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kerberos-io/agent/machinery/src/models"
)

func TestS3RecordingUploadDetailsUsesCheckpoint(t *testing.T) {
	for _, fileName := range []string{
		"1767355200123-checkpoint-key.mp4",
		"1564859471_6-474162_old-camera_0-0-0-0_1153_27.mp4",
	} {
		t.Run(fileName, func(t *testing.T) {
			withRecording(t, fileName, []byte("recording"))
			recorded := time.Date(2026, time.January, 2, 12, 0, 0, 123000000, time.UTC)
			metadata := models.RecordingUploadMetadata{
				FileName:          fileName,
				DeviceKey:         "checkpoint-key",
				Timestamp:         recorded.UnixMilli(),
				Duration:          20452,
				FPS:               29.97,
				Encrypted:         true,
				DeviceName:        "checkpoint-camera",
				RegionCoordinates: "1-2-3-4",
				NumberOfChanges:   "57",
			}
			payload, err := json.Marshal(metadata)
			if err != nil {
				t.Fatal(err)
			}
			withQueuedRecordingFPS(t, fileName, string(payload))
			config := models.Config{
				Key:  "changed-config-key",
				Name: "camera-name",
				S3:   &models.S3{Username: "user"},
			}
			objectName, userMetadata, err := s3RecordingUploadDetails(config, fileName, "public-key")
			if err != nil {
				t.Fatal(err)
			}
			wantObjectName := "user/2026/01/02/" + strconv.FormatInt(recorded.UnixMilli(), 10) + "-checkpoint-key.mp4"
			if objectName != wantObjectName {
				t.Fatalf("object name = %q, want %q", objectName, wantObjectName)
			}
			wantMetadata := map[string]string{
				"publickey":               "public-key",
				"uploadtime":              "now",
				"event-instancename":      "checkpoint-camera",
				"event-regioncoordinates": "1-2-3-4",
				"event-numberofchanges":   "57",
				"event-microseconds":      "123000",
				"event-timestamp":         strconv.FormatInt(recorded.Unix(), 10),
				"productid":               "checkpoint-key",
				"duration":                "20452",
				"fps":                     "29.97",
				"encrypted":               "true",
			}
			if !reflect.DeepEqual(userMetadata, wantMetadata) {
				t.Fatalf("metadata = %#v, want %#v", userMetadata, wantMetadata)
			}
		})
	}
}

func TestS3RecordingUploadDetailsBackfillsLegacyCheckpointDescriptions(t *testing.T) {
	const legacyName = "1564859471_6-474162_recorded-camera_1-2-3-4_1153_27.mp4"
	tests := []struct {
		name        string
		fileName    string
		camera      string
		region      string
		changes     string
		wantCamera  string
		wantRegion  string
		wantChanges string
	}{
		{"pre-upgrade checkpoint", legacyName, "", "", "", "recorded-camera", "1-2-3-4", "1153"},
		{"checkpoint camera", legacyName, "snapshot-camera", "", "", "snapshot-camera", "1-2-3-4", "1153"},
		{"checkpoint region", legacyName, "", "5-6-7-8", "", "recorded-camera", "5-6-7-8", "1153"},
		{"checkpoint zero changes", legacyName, "", "", "0", "recorded-camera", "1-2-3-4", "0"},
		{"dated reference", "2019/08/03/" + legacyName, "", "", "", "recorded-camera", "1-2-3-4", "1153"},
		{"continuous recording", "1564859471_3-474_recorded-camera_0-0-0-0_-1_27.mp4", "", "", "", "recorded-camera", "0-0-0-0", "-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withRecording(t, filepath.Base(tt.fileName), []byte("recording"))
			metadata := models.RecordingUploadMetadata{
				FileName:          filepath.Base(tt.fileName),
				DeviceKey:         "checkpoint-key",
				Timestamp:         1767355200123,
				Duration:          20452,
				FPS:               29.97,
				Encrypted:         true,
				DeviceName:        tt.camera,
				RegionCoordinates: tt.region,
				NumberOfChanges:   tt.changes,
			}
			payload, err := json.Marshal(metadata)
			if err != nil {
				t.Fatal(err)
			}
			withQueuedRecordingFPS(t, tt.fileName, string(payload))
			config := models.Config{Key: "current-key", Name: "current-camera", S3: &models.S3{Username: "user"}}
			objectName, got, err := s3RecordingUploadDetails(config, tt.fileName, "public-key")
			if err != nil {
				t.Fatal(err)
			}
			if want := "user/2026/01/02/1767355200123-checkpoint-key.mp4"; objectName != want {
				t.Fatalf("object name = %q, want %q", objectName, want)
			}
			want := map[string]string{
				"publickey":               "public-key",
				"uploadtime":              "now",
				"event-instancename":      tt.wantCamera,
				"event-regioncoordinates": tt.wantRegion,
				"event-numberofchanges":   tt.wantChanges,
				"event-microseconds":      "123000",
				"event-timestamp":         "1767355200",
				"productid":               "checkpoint-key",
				"duration":                "20452",
				"fps":                     "29.97",
				"encrypted":               "true",
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("metadata = %#v, want %#v", got, want)
			}
		})
	}
}

func TestS3RecordingUploadDetailsDoesNotBackfillInvalidLegacyNames(t *testing.T) {
	for _, fileName := range []string{
		"1767355200123-checkpoint-key.mp4",
		"invalid_6-474162_camera_0-0-0-0_1153_27.mp4",
		"1564859471_6-invalid_camera_0-0-0-0_1153_27.mp4",
		"1564859471_3-474162_camera_0-0-0-0_1153_27.mp4",
		"1564859471_6-474162__0-0-0-0_1153_27.mp4",
		"1564859471_6-474162_camera_invalid_1153_27.mp4",
		"1564859471_6-474162_camera_0-0-0-0_invalid_27.mp4",
		"1564859471_6-474162_camera_0-0-0-0_1153_invalid.mp4",
		"1564859471_6-474162_camera_0-0-0-0_1153_27_extra.mp4",
	} {
		t.Run(fileName, func(t *testing.T) {
			withRecording(t, fileName, []byte("recording"))
			metadata := models.RecordingUploadMetadata{
				FileName: fileName, DeviceKey: "checkpoint-key", Timestamp: 1767355200123, Duration: 20452,
			}
			payload, err := json.Marshal(metadata)
			if err != nil {
				t.Fatal(err)
			}
			withQueuedRecordingFPS(t, fileName, string(payload))
			config := models.Config{Key: "current-key", Name: "current-camera", S3: &models.S3{Username: "user"}}
			objectName, got, err := s3RecordingUploadDetails(config, fileName, "public-key")
			if err != nil {
				t.Fatal(err)
			}
			if objectName != "user/2026/01/02/1767355200123-checkpoint-key.mp4" ||
				got["event-instancename"] != "current-camera" || got["event-regioncoordinates"] != "" ||
				got["event-numberofchanges"] != "" || got["event-timestamp"] != "1767355200" ||
				got["productid"] != "checkpoint-key" {
				t.Fatalf("unexpected fallback for invalid legacy name: %q, %#v", objectName, got)
			}
		})
	}
}

func TestS3RecordingUploadDetailsLegacyMarkers(t *testing.T) {
	for _, marker := range []string{"missing", "empty", "fps-only", "old-marker-name"} {
		t.Run(marker, func(t *testing.T) {
			const fileName = "1564859471_6-474162_camera_0-0-0-0_1153_27.mp4"
			withRecording(t, fileName, []byte("recording"))
			switch marker {
			case "empty":
				withQueuedRecordingFPS(t, fileName, "")
			case "fps-only", "old-marker-name":
				withQueuedRecordingFPS(t, fileName, "29.97")
				if marker == "old-marker-name" {
					err := os.Rename(filepath.Join("data/cloud", models.RecordingUploadMetadataFileName(fileName)),
						filepath.Join("data/cloud", fileName))
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			config := models.Config{Key: "legacy-key", S3: &models.S3{Username: "user"}}
			objectName, metadata, err := s3RecordingUploadDetails(config, fileName, "public-key")
			if err != nil {
				t.Fatal(err)
			}
			if want := "user/2019/08/03/1564859471474-legacy-key.mp4"; objectName != want {
				t.Fatalf("object name = %q, want %q", objectName, want)
			}
			if metadata["productid"] != "legacy-key" || metadata["event-instancename"] != "camera" ||
				metadata["event-regioncoordinates"] != "0-0-0-0" || metadata["event-timestamp"] != "1564859471" {
				t.Fatalf("unexpected legacy metadata: %#v", metadata)
			}
		})
	}
}

func TestS3RecordingUploadDetailsRejectsInvalidCheckpoint(t *testing.T) {
	tests := []struct {
		name   string
		marker string
		want   string
	}{
		{"malformed JSON", `{"timestamp":`, "decode"},
		{"missing timestamp", `{"device_key":"camera"}`, "timestamp"},
		{"missing device key", `{"timestamp":1787015373000}`, "device key"},
		{"invalid device key", `{"timestamp":1787015373000,"device_key":"camera/key"}`, "device key"},
		{"mismatched filename", `{"timestamp":1787015373000,"device_key":"camera","filename":"other.mp4"}`, "filename"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const fileName = "1564859471_6-474162_camera_0-0-0-0_1153_27.mp4"
			withRecording(t, fileName, []byte("recording"))
			withQueuedRecordingFPS(t, fileName, tt.marker)
			config := models.Config{Key: "legacy-key", S3: &models.S3{Username: "user"}}
			_, _, err := s3RecordingUploadDetails(config, fileName, "public-key")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want error containing %q instead of legacy fallback", err, tt.want)
			}
		})
	}
}

func TestS3RecordingUploadDetailsReportsCheckpointReadError(t *testing.T) {
	const fileName = "1564859471_6-474162_camera_0-0-0-0_1153_27.mp4"
	withRecording(t, fileName, []byte("recording"))
	if err := os.MkdirAll(filepath.Join("data/cloud", models.RecordingUploadMetadataFileName(fileName)), 0o755); err != nil {
		t.Fatal(err)
	}
	config := models.Config{Key: "legacy-key", S3: &models.S3{Username: "user"}}
	_, _, err := s3RecordingUploadDetails(config, fileName, "public-key")
	if err == nil || !strings.Contains(err.Error(), "read recording upload metadata") {
		t.Fatalf("error = %v, want checkpoint read error instead of legacy fallback", err)
	}
}

func TestS3RecordingObjectName(t *testing.T) {
	tests := []struct {
		name      string
		recorded  time.Time
		directory string
	}{
		{
			name:      "zero padded month and day",
			recorded:  time.Date(2026, time.January, 2, 12, 0, 0, 0, time.UTC),
			directory: "2026/01/02",
		},
		{
			name:      "UTC date across year boundary",
			recorded:  time.Date(2026, time.January, 1, 1, 0, 0, 0, time.FixedZone("UTC+2", 2*60*60)),
			directory: "2025/12/31",
		},
		{
			name:      "leap day",
			recorded:  time.Date(2024, time.February, 29, 23, 59, 59, 0, time.UTC),
			directory: "2024/02/29",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const deviceKey = "camera-key-123"
			want := "user/" + tt.directory + "/" + strconv.FormatInt(tt.recorded.UnixMilli(), 10) + "-" + deviceKey + ".mp4"
			for attempt := 0; attempt < 2; attempt++ {
				if got := s3RecordingObjectName("user", deviceKey, tt.recorded.UnixMilli()); got != want {
					t.Fatalf("s3RecordingObjectName() = %q, want %q", got, want)
				}
			}
		})
	}
}

func TestUploadS3RejectsInvalidRecordingNames(t *testing.T) {
	configuration := &models.Configuration{
		Config: models.Config{
			Key: "camera-key-123",
			S3: &models.S3{
				Publickey: "test-public-key",
				Secretkey: "test-secret-key",
				Region:    "eu-west-1",
			},
		},
	}
	for _, fileName := range []string{
		"recording.mp4",
		"1787015373_camera.mp4",
		"invalid_6-474162_camera_0-0-0-0_1153_27.mp4",
	} {
		t.Run(fileName, func(t *testing.T) {
			uploaded, configured, err := UploadS3(configuration, fileName)
			if uploaded || !configured || err == nil {
				t.Fatalf("UploadS3() = (%v, %v, %v), want (false, true, error)", uploaded, configured, err)
			}
		})
	}
}

func TestUploadS3RejectsInvalidDeviceKeys(t *testing.T) {
	for _, deviceKey := range []string{"", "camera/key", `camera\key`} {
		t.Run(deviceKey, func(t *testing.T) {
			configuration := &models.Configuration{
				Config: models.Config{
					Key: deviceKey,
					S3: &models.S3{
						Publickey: "test-public-key",
						Secretkey: "test-secret-key",
						Region:    "eu-west-1",
					},
				},
			}
			uploaded, configured, err := UploadS3(configuration, "1787015373_6-474162_camera_0-0-0-0_1153_27.mp4")
			if uploaded || !configured || err == nil || !strings.Contains(err.Error(), "device key") {
				t.Fatalf("UploadS3() = (%v, %v, %v), want (false, true, device key error)", uploaded, configured, err)
			}
		})
	}
}
