package config

import (
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/kerberos-io/agent/machinery/src/models"
)

// Every AGENT_* variable handled by applyAgentEnvVars needs an entry in
// envConfigFields, and the table must not list variables that are not handled.
func TestEnvConfigFieldsCoverEveryEnvVar(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(source), "func applyAgentEnvVars(")
	if start < 0 {
		t.Fatal("applyAgentEnvVars not found in main.go")
	}
	handled := map[string]bool{}
	for _, match := range regexp.MustCompile(`case "(AGENT_[A-Z0-9_]+)":`).FindAllStringSubmatch(string(source[start:]), -1) {
		handled[match[1]] = true
	}
	if len(handled) == 0 {
		t.Fatal("no AGENT_* cases found")
	}
	for key := range handled {
		if _, ok := envConfigFields[key]; !ok {
			t.Errorf("%s is handled by applyAgentEnvVars but missing from envConfigFields", key)
		}
	}
	for key := range envConfigFields {
		if !handled[key] {
			t.Errorf("%s is in envConfigFields but not handled by applyAgentEnvVars", key)
		}
	}
}

func TestEnvConfigFieldsResolveToJSONPaths(t *testing.T) {
	for key, goPath := range envConfigFields {
		if _, ok := configJSONPath(goPath); !ok {
			t.Errorf("%s: %q is not a field path of models.Config", key, goPath)
		}
	}
	want := map[string]string{
		"AGENT_CAPTURE_IPCAMERA_RTSP":       "capture.ipcamera.rtsp",
		"AGENT_CAPTURE_PIXEL_CHANGE":        "capture.pixelChangeThreshold",
		"AGENT_REGION_POLYGON":              "region.polygon",
		"AGENT_KERBEROSVAULT_SECONDARY_URI": "kstorage_secondary.uri",
		"AGENT_NAME":                        "friendly_name",
		"AGENT_HUB_REGION":                  "s3.region",
	}
	for key, path := range want {
		got, _ := configJSONPath(envConfigFields[key])
		if got != path {
			t.Errorf("%s: path = %q, want %q", key, got, path)
		}
	}
}

// Setting a variable on its own must change the field the table names.
func TestEnvConfigFieldsMatchApplyAgentEnvVars(t *testing.T) {
	samples := map[string]string{
		"AGENT_TIMETABLE":      "0,43199,43200,86400",
		"AGENT_REGION_POLYGON": "0,0;10,0;10,10",
	}
	for key, goPath := range envConfigFields {
		t.Run(key, func(t *testing.T) {
			configuration := &models.Configuration{}
			initConfigPointers(&configuration.Config)
			field := configField(reflect.ValueOf(&configuration.Config).Elem(), goPath)
			if !field.IsValid() {
				t.Fatalf("%q not found", goPath)
			}
			sample, ok := samples[key]
			if !ok {
				switch underlying(field.Type()).Kind() {
				case reflect.String:
					sample = "sample"
				default:
					sample = "7"
				}
			}
			t.Setenv(key, sample)

			applyAgentEnvVars(configuration, "", false)

			field = configField(reflect.ValueOf(&configuration.Config).Elem(), goPath)
			if !field.IsValid() || field.IsZero() {
				t.Fatalf("%s=%q did not set %s", key, sample, goPath)
			}
		})
	}
}

func TestEnvManagedConfig(t *testing.T) {
	clearAgentEnv(t)

	t.Run("nothing set", func(t *testing.T) {
		if got := EnvManagedConfig(); !got.IsEmpty() {
			t.Fatalf("got %+v, want empty", got)
		}
	})

	t.Run("standalone with AGENT_* variables", func(t *testing.T) {
		t.Setenv("AGENT_CAPTURE_IPCAMERA_RTSP", "rtsp://camera/stream")
		t.Setenv("AGENT_REGION_POLYGON", "0,0;10,0;10,10")
		t.Setenv("DEPLOYMENT_NAME", "agent-front-door")
		got := EnvManagedConfig()
		want := EnvManaged{All: false, Paths: []string{"capture.ipcamera.rtsp", "name", "region.polygon"}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %+v, want %+v", got, want)
		}
	})

	t.Run("ConfigMap mode manages everything", func(t *testing.T) {
		t.Setenv("GLOBAL_AGENT_TIMEZONE", "Europe/Brussels")
		t.Setenv("AGENT_CAPTURE_IPCAMERA_RTSP", "rtsp://camera/stream")
		got := EnvManagedConfig()
		want := EnvManaged{All: true, Paths: []string{"capture.ipcamera.rtsp", "timezone"}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %+v, want %+v", got, want)
		}
	})

	t.Run("Factory mode ignores GLOBAL_AGENT_* variables", func(t *testing.T) {
		t.Setenv("DEPLOYMENT", "factory")
		t.Setenv("GLOBAL_AGENT_TIMEZONE", "Europe/Brussels")
		t.Setenv("AGENT_CAPTURE_IPCAMERA_RTSP", "rtsp://camera/stream")
		got := EnvManagedConfig()
		want := EnvManaged{All: false, Paths: []string{"capture.ipcamera.rtsp"}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %+v, want %+v", got, want)
		}
	})
}

// clearAgentEnv unsets variables that would leak into these tests from the
// environment running them; t.Setenv restores them afterwards.
func clearAgentEnv(t *testing.T) {
	for _, env := range os.Environ() {
		key := strings.SplitN(env, "=", 2)[0]
		if strings.HasPrefix(key, "AGENT_") || strings.HasPrefix(key, "GLOBAL_AGENT_") ||
			key == "DEPLOYMENT" || key == "DEPLOYMENT_NAME" {
			t.Setenv(key, "")
			os.Unsetenv(key)
		}
	}
}

func underlying(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}

// configField follows a Go field path through (pointer) structs.
func configField(v reflect.Value, goPath string) reflect.Value {
	for _, segment := range strings.Split(goPath, ".") {
		for v.Kind() == reflect.Ptr {
			if v.IsNil() {
				return reflect.Value{}
			}
			v = v.Elem()
		}
		v = v.FieldByName(segment)
		if !v.IsValid() {
			return v
		}
	}
	return v
}
