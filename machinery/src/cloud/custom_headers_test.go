package cloud

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
)

func TestCustomVaultHeadersAreValidatedAndAdded(t *testing.T) {
	customHeaders, err := parseCustomVaultHeaders(`{"site_id":"site-1","line_id":"line-2"}`)
	if err != nil {
		t.Fatal(err)
	}

	headers := http.Header{}
	if err := setCustomVaultHeaders(headers, customHeaders); err != nil {
		t.Fatal(err)
	}
	if headers.Get("site_id") != "site-1" || headers.Get("line_id") != "line-2" {
		t.Fatalf("custom headers = %#v", headers)
	}

	encodedManifest, err := base64.RawURLEncoding.DecodeString(headers.Get(customHeadersManifestHeader))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	if err := json.Unmarshal(encodedManifest, &names); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(names, []string{"line_id", "site_id"}) {
		t.Fatalf("manifest = %#v", names)
	}
}

func TestCustomVaultHeadersRejectReservedAndInvalidValues(t *testing.T) {
	for _, value := range []string{
		`{"X-Kerberos-Storage-AccessKey":"secret"}`,
		`{"Authorization":"secret"}`,
		`{"site_id":"one\nInjected: value"}`,
		`{"bad header":"value"}`,
		`{"site_id":1}`,
		`{"Content-Type":"text/plain"}`,
		`{"site_id":"one","SITE_ID":"two"}`,
		`null`,
	} {
		if _, err := parseCustomVaultHeaders(value); err == nil {
			t.Fatalf("parseCustomVaultHeaders(%q) succeeded", value)
		}
	}
}
