package cloud

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

const (
	customHeadersManifestHeader = "X-Kerberos-Storage-Custom-Headers"
	maxCustomHeaders            = 32
	maxCustomHeaderNameLength   = 128
	maxCustomHeaderValueLength  = 4096
)

var disallowedCustomHeaders = map[string]struct{}{
	"authorization":       {},
	"connection":          {},
	"content-length":      {},
	"content-type":        {},
	"cookie":              {},
	"expect":              {},
	"host":                {},
	"keep-alive":          {},
	"proxy-authorization": {},
	"proxy-connection":    {},
	"te":                  {},
	"trailer":             {},
	"trace-id":            {},
	"transfer-encoding":   {},
	"tus-resumable":       {},
	"upgrade":             {},
}

func parseCustomVaultHeaders(value string) (map[string]string, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}

	// Decode into pointers so JSON null values can be told apart from strings.
	var rawHeaders map[string]*string
	if err := json.Unmarshal([]byte(value), &rawHeaders); err != nil {
		return nil, fmt.Errorf("custom Vault headers must be a JSON object of string values: %w", err)
	}
	if rawHeaders == nil {
		return nil, errors.New("custom Vault headers must be a JSON object of string values")
	}
	if len(rawHeaders) > maxCustomHeaders {
		return nil, fmt.Errorf("custom Vault headers exceed the limit of %d", maxCustomHeaders)
	}
	headers := make(map[string]string, len(rawHeaders))
	seen := make(map[string]struct{}, len(rawHeaders))
	for name, rawValue := range rawHeaders {
		if rawValue == nil {
			return nil, fmt.Errorf("custom Vault header %q must be a string, not null", name)
		}
		value := *rawValue
		headers[name] = value
		if err := validateCustomVaultHeader(name, value); err != nil {
			return nil, err
		}
		lowerName := strings.ToLower(name)
		if _, duplicate := seen[lowerName]; duplicate {
			return nil, fmt.Errorf("custom Vault header %q is duplicated with different casing", name)
		}
		seen[lowerName] = struct{}{}
	}
	return headers, nil
}

func validateCustomVaultHeader(name, value string) error {
	if name == "" || len(name) > maxCustomHeaderNameLength || !isHTTPToken(name) {
		return fmt.Errorf("invalid custom Vault header name %q", name)
	}
	lowerName := strings.ToLower(name)
	if strings.HasPrefix(lowerName, "x-kerberos-") {
		return fmt.Errorf("custom Vault header %q uses the reserved X-Kerberos namespace", name)
	}
	if strings.HasPrefix(lowerName, "upload-") {
		return fmt.Errorf("custom Vault header %q uses the reserved tus Upload namespace", name)
	}
	if _, disallowed := disallowedCustomHeaders[lowerName]; disallowed {
		return fmt.Errorf("custom Vault header %q is reserved", name)
	}
	if len(value) > maxCustomHeaderValueLength {
		return fmt.Errorf("custom Vault header %q exceeds the %d byte value limit", name, maxCustomHeaderValueLength)
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return fmt.Errorf("custom Vault header %q contains an invalid value", name)
		}
	}
	return nil
}

func isHTTPToken(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			strings.ContainsRune("!#$%&'*+-.^_`|~", character) {
			continue
		}
		return false
	}
	return true
}

func setCustomVaultHeaders(headers http.Header, customHeaders map[string]string) error {
	if len(customHeaders) == 0 {
		return nil
	}

	names := make([]string, 0, len(customHeaders))
	for name, value := range customHeaders {
		if err := validateCustomVaultHeader(name, value); err != nil {
			return err
		}
		names = append(names, name)
		// Assign directly so the configured name is not canonicalised by Set.
		headers[name] = []string{value}
	}
	sort.Strings(names)
	manifest, err := json.Marshal(names)
	if err != nil {
		return err
	}
	headers.Set(customHeadersManifestHeader, base64.RawURLEncoding.EncodeToString(manifest))
	return nil
}

const maxVaultRedirects = 10

// stripVaultHeadersOnCrossHostRedirect removes Vault credentials and the
// customer-supplied metadata (custom headers, their manifest and the tus
// Upload-Metadata that embeds them) before following a redirect to another host.
func stripVaultHeadersOnCrossHostRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxVaultRedirects {
		return fmt.Errorf("stopped after %d redirects", maxVaultRedirects)
	}
	if len(via) == 0 || req.URL.Host == via[0].URL.Host {
		return nil
	}

	customNames := customVaultHeaderNamesFromManifest(req.Header)
	for name := range req.Header {
		canonicalName := http.CanonicalHeaderKey(name)
		_, custom := customNames[strings.ToLower(name)]
		if custom || strings.HasPrefix(canonicalName, "X-Kerberos-") || canonicalName == "Upload-Metadata" {
			delete(req.Header, name)
		}
	}
	return nil
}

func customVaultHeaderNamesFromManifest(headers http.Header) map[string]struct{} {
	encoded := headers.Get(customHeadersManifestHeader)
	if encoded == "" {
		return nil
	}
	manifest, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil
	}
	var names []string
	if err := json.Unmarshal(manifest, &names); err != nil {
		return nil
	}
	lowerNames := make(map[string]struct{}, len(names))
	for _, name := range names {
		lowerNames[strings.ToLower(name)] = struct{}{}
	}
	return lowerNames
}

func encodeCustomVaultHeaders(customHeaders map[string]string) (string, error) {
	if len(customHeaders) == 0 {
		return "", nil
	}
	encoded, err := json.Marshal(customHeaders)
	if err != nil {
		return "", err
	}
	if len(encoded) == 0 {
		return "", errors.New("custom Vault headers could not be encoded")
	}
	return string(encoded), nil
}
