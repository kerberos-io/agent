package cloud

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kerberos-io/agent/machinery/src/models"
)

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
			want := "user/" + tt.directory + "/" + strconv.FormatInt(tt.recorded.Unix(), 10) + "-" + deviceKey + ".mp4"
			for attempt := 0; attempt < 2; attempt++ {
				if got := s3RecordingObjectName("user", deviceKey, tt.recorded.Unix()); got != want {
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
