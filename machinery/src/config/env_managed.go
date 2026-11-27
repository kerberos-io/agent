package config

import (
	"os"
	"reflect"
	"sort"
	"strings"

	"github.com/kerberos-io/agent/machinery/src/models"
)

// envConfigFields maps every AGENT_* environment variable that
// applyAgentEnvVars reads to the Config field it sets, as a Go field path.
// TestEnvConfigFieldsCoverEveryEnvVar keeps this table in step with the
// switch in applyAgentEnvVars.
var envConfigFields = map[string]string{
	"AGENT_KEY":                                    "Key",
	"AGENT_NAME":                                   "FriendlyName",
	"AGENT_TIMEZONE":                               "Timezone",
	"AGENT_OFFLINE":                                "Offline",
	"AGENT_AUTO_CLEAN":                             "AutoClean",
	"AGENT_AUTO_CLEAN_MAX_SIZE":                    "MaxDirectorySize",
	"AGENT_AUTO_CLEAN_MIN_FREE_SPACE":              "MinFreeSpace",
	"AGENT_CAPTURE_IPCAMERA_RTSP":                  "Capture.IPCamera.RTSP",
	"AGENT_CAPTURE_IPCAMERA_RTSP_TRANSPORT":        "Capture.IPCamera.RTSPTransport",
	"AGENT_CAPTURE_IPCAMERA_SUB_RTSP":              "Capture.IPCamera.SubRTSP",
	"AGENT_CAPTURE_IPCAMERA_BASE_WIDTH":            "Capture.IPCamera.BaseWidth",
	"AGENT_CAPTURE_IPCAMERA_BASE_HEIGHT":           "Capture.IPCamera.BaseHeight",
	"AGENT_CAPTURE_IPCAMERA_ONVIF":                 "Capture.IPCamera.ONVIF",
	"AGENT_CAPTURE_IPCAMERA_ONVIF_XADDR":           "Capture.IPCamera.ONVIFXAddr",
	"AGENT_CAPTURE_IPCAMERA_ONVIF_USERNAME":        "Capture.IPCamera.ONVIFUsername",
	"AGENT_CAPTURE_IPCAMERA_ONVIF_PASSWORD":        "Capture.IPCamera.ONVIFPassword",
	"AGENT_CAPTURE_IPCAMERA_ONVIF_CHANNEL":         "Capture.IPCamera.ONVIFChannel",
	"AGENT_CAPTURE_RECORDING":                      "Capture.Recording",
	"AGENT_CAPTURE_CONTINUOUS":                     "Capture.Continuous",
	"AGENT_CAPTURE_LIVEVIEW":                       "Capture.Liveview",
	"AGENT_CAPTURE_MOTION":                         "Capture.Motion",
	"AGENT_CAPTURE_ONVIF_MOTION":                   "Capture.ONVIFMotion",
	"AGENT_CAPTURE_SNAPSHOTS":                      "Capture.Snapshots",
	"AGENT_CAPTURE_PRERECORDING":                   "Capture.PreRecording",
	"AGENT_CAPTURE_POSTRECORDING":                  "Capture.PostRecording",
	"AGENT_CAPTURE_MAXLENGTH":                      "Capture.MaxLengthRecording",
	"AGENT_CAPTURE_PIXEL_CHANGE":                   "Capture.PixelChangeThreshold",
	"AGENT_CAPTURE_FRAGMENTED":                     "Capture.Fragmented",
	"AGENT_CAPTURE_FRAGMENTED_DURATION":            "Capture.FragmentedDuration",
	"AGENT_CAPTURE_LIVEVIEW_CHUNKING":              "Capture.LiveviewChunking",
	"AGENT_TIME":                                   "Time",
	"AGENT_TIMETABLE":                              "Timetable",
	"AGENT_REGION_POLYGON":                         "Region.Polygon",
	"AGENT_MQTT_URI":                               "MQTTURI",
	"AGENT_MQTT_USERNAME":                          "MQTTUsername",
	"AGENT_MQTT_PASSWORD":                          "MQTTPassword",
	"AGENT_REALTIME_PROCESSING":                    "RealtimeProcessing",
	"AGENT_REALTIME_PROCESSING_TOPIC":              "RealtimeProcessingTopic",
	"AGENT_STUN_URI":                               "STUNURI",
	"AGENT_FORCE_TURN":                             "ForceTurn",
	"AGENT_TURN_URI":                               "TURNURI",
	"AGENT_TURN_USERNAME":                          "TURNUsername",
	"AGENT_TURN_PASSWORD":                          "TURNPassword",
	"AGENT_CLOUD":                                  "Cloud",
	"AGENT_REMOVE_AFTER_UPLOAD":                    "RemoveAfterUpload",
	"AGENT_HUB_ENCRYPTION":                         "HubEncryption",
	"AGENT_HUB_URI":                                "HubURI",
	"AGENT_HUB_KEY":                                "HubKey",
	"AGENT_HUB_PRIVATE_KEY":                        "HubPrivateKey",
	"AGENT_HUB_SITE":                               "HubSite",
	"AGENT_HUB_REGION":                             "S3.Region",
	"AGENT_KERBEROSVAULT_URI":                      "KStorage.URI",
	"AGENT_KERBEROSVAULT_ACCESS_KEY":               "KStorage.AccessKey",
	"AGENT_KERBEROSVAULT_SECRET_KEY":               "KStorage.SecretAccessKey",
	"AGENT_KERBEROSVAULT_PROVIDER":                 "KStorage.Provider",
	"AGENT_KERBEROSVAULT_DIRECTORY":                "KStorage.Directory",
	"AGENT_KERBEROSVAULT_CUSTOM_HEADERS":           "KStorage.CustomHeaders",
	"AGENT_KERBEROSVAULT_MAX_RETRIES":              "KStorage.MaxRetries",
	"AGENT_KERBEROSVAULT_TIMEOUT":                  "KStorage.Timeout",
	"AGENT_KERBEROSVAULT_SECONDARY_URI":            "KStorageSecondary.URI",
	"AGENT_KERBEROSVAULT_SECONDARY_ACCESS_KEY":     "KStorageSecondary.AccessKey",
	"AGENT_KERBEROSVAULT_SECONDARY_SECRET_KEY":     "KStorageSecondary.SecretAccessKey",
	"AGENT_KERBEROSVAULT_SECONDARY_PROVIDER":       "KStorageSecondary.Provider",
	"AGENT_KERBEROSVAULT_SECONDARY_DIRECTORY":      "KStorageSecondary.Directory",
	"AGENT_KERBEROSVAULT_SECONDARY_CUSTOM_HEADERS": "KStorageSecondary.CustomHeaders",
	"AGENT_DROPBOX_ACCESS_TOKEN":                   "Dropbox.AccessToken",
	"AGENT_DROPBOX_DIRECTORY":                      "Dropbox.Directory",
	"AGENT_ENCRYPTION":                             "Encryption.Enabled",
	"AGENT_ENCRYPTION_RECORDINGS":                  "Encryption.Recordings",
	"AGENT_ENCRYPTION_FINGERPRINT":                 "Encryption.Fingerprint",
	"AGENT_ENCRYPTION_PRIVATE_KEY":                 "Encryption.PrivateKey",
	"AGENT_ENCRYPTION_SYMMETRIC_KEY":               "Encryption.SymmetricKey",
	"AGENT_SIGNING":                                "Signing.Enabled",
	"AGENT_SIGNING_PRIVATE_KEY":                    "Signing.PrivateKey",
}

// EnvManaged tells a remote editor (the Hub) which settings come from
// environment variables. The agent applies those again on every start, so a
// change saved to them through the Hub is undone at the next restart.
//
// All is true in ConfigMap mode: the configuration is then built only from
// GLOBAL_AGENT_* and AGENT_* variables and the saved config.json is ignored,
// so no remote change survives a restart. Paths lists the JSON paths of the
// configuration fields set by environment variables, e.g.
// "capture.ipcamera.rtsp".
type EnvManaged struct {
	All   bool     `json:"all"`
	Paths []string `json:"paths"`
}

// IsEmpty reports whether no setting is managed by environment variables.
func (e EnvManaged) IsEmpty() bool {
	return !e.All && len(e.Paths) == 0
}

// EnvManagedConfig reports which settings the current environment variables
// manage, following the same rules as OverrideWithEnvironmentVariables.
func EnvManagedConfig() EnvManaged {
	standalone := os.Getenv("DEPLOYMENT") == "" || os.Getenv("DEPLOYMENT") == "agent"

	seen := map[string]bool{}
	add := func(goPath string) {
		if path, ok := configJSONPath(goPath); ok {
			seen[path] = true
		}
	}

	if os.Getenv("DEPLOYMENT_NAME") != "" {
		add("Name")
	}
	for _, env := range os.Environ() {
		fullKey := strings.SplitN(env, "=", 2)[0]
		key := fullKey
		if strings.HasPrefix(fullKey, "GLOBAL_AGENT_") {
			// GLOBAL_AGENT_* variables are only applied in standalone and
			// ConfigMap mode, not in Factory/MongoDB mode.
			if !standalone {
				continue
			}
			key = strings.TrimPrefix(fullKey, "GLOBAL_")
		}
		if goPath, ok := envConfigFields[key]; ok {
			add(goPath)
		}
	}

	paths := make([]string, 0, len(seen))
	for path := range seen {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	return EnvManaged{
		All:   standalone && isConfigMapMode(),
		Paths: paths,
	}
}

// configJSONPath converts a Go field path of models.Config (e.g.
// "Capture.IPCamera.RTSP") into its JSON path (e.g. "capture.ipcamera.rtsp").
func configJSONPath(goPath string) (string, bool) {
	t := reflect.TypeOf(models.Config{})
	segments := strings.Split(goPath, ".")
	jsonSegments := make([]string, 0, len(segments))
	for _, segment := range segments {
		for t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		if t.Kind() != reflect.Struct {
			return "", false
		}
		field, ok := t.FieldByName(segment)
		if !ok {
			return "", false
		}
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name == "" || name == "-" {
			name = field.Name
		}
		jsonSegments = append(jsonSegments, name)
		t = field.Type
	}
	return strings.Join(jsonSegments, "."), true
}
