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
	if !reflect.DeepEqual(headers["site_id"], []string{"site-1"}) || !reflect.DeepEqual(headers["line_id"], []string{"line-2"}) {
		t.Fatalf("custom header names were not preserved: %#v", headers)
	}
	if _, canonicalised := headers["Site_id"]; canonicalised {
		t.Fatalf("custom header name was canonicalised: %#v", headers)
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
		`{"site_id":null}`,
		`{"site_id":"one\ttwo"}`,
		`{"Upload-Checksum":"sha1 abc"}`,
		`{"upload-concat":"final;/files/a"}`,
		`{"Upload-Defer-Length":"1"}`,
		`null`,
	} {
		if _, err := parseCustomVaultHeaders(value); err == nil {
			t.Fatalf("parseCustomVaultHeaders(%q) succeeded", value)
		}
	}
}

func TestVaultRedirectStripsCustomHeadersOnHostChange(t *testing.T) {
	newRedirect := func(t *testing.T, host string) (*http.Request, []*http.Request) {
		t.Helper()
		original, err := http.NewRequest(http.MethodPost, "https://vault.example.com/api/storage", nil)
		if err != nil {
			t.Fatal(err)
		}
		redirected, err := http.NewRequest(http.MethodPost, "https://"+host+"/api/storage", nil)
		if err != nil {
			t.Fatal(err)
		}
		redirected.Header.Set("X-Kerberos-Storage-AccessKey", "secret")
		redirected.Header.Set("Upload-Metadata", "custom_headers abc")
		redirected.Header.Set("Tus-Resumable", "1.0.0")
		if err := setCustomVaultHeaders(redirected.Header, map[string]string{"site_id": "site-1", "Line-Id": "line-2"}); err != nil {
			t.Fatal(err)
		}
		return redirected, []*http.Request{original}
	}

	crossHost, via := newRedirect(t, "other.example.com")
	if err := stripVaultHeadersOnCrossHostRedirect(crossHost, via); err != nil {
		t.Fatal(err)
	}
	if len(crossHost.Header) != 1 || crossHost.Header.Get("Tus-Resumable") != "1.0.0" {
		t.Fatalf("cross-host redirect headers = %#v", crossHost.Header)
	}

	sameHost, via := newRedirect(t, "vault.example.com")
	if err := stripVaultHeadersOnCrossHostRedirect(sameHost, via); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sameHost.Header["site_id"], []string{"site-1"}) ||
		sameHost.Header.Get("X-Kerberos-Storage-AccessKey") != "secret" ||
		sameHost.Header.Get(customHeadersManifestHeader) == "" {
		t.Fatalf("same-host redirect headers = %#v", sameHost.Header)
	}
}
