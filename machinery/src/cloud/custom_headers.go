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
	"upload-length":       {},
	"upload-metadata":     {},
	"upload-offset":       {},
}

func parseCustomVaultHeaders(value string) (map[string]string, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}

	var headers map[string]string
	if err := json.Unmarshal([]byte(value), &headers); err != nil {
		return nil, fmt.Errorf("custom Vault headers must be a JSON object of string values: %w", err)
	}
	if headers == nil {
		return nil, errors.New("custom Vault headers must be a JSON object of string values")
	}
	if len(headers) > maxCustomHeaders {
		return nil, fmt.Errorf("custom Vault headers exceed the limit of %d", maxCustomHeaders)
	}
	seen := make(map[string]struct{}, len(headers))
	for name, value := range headers {
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
	if _, disallowed := disallowedCustomHeaders[lowerName]; disallowed {
		return fmt.Errorf("custom Vault header %q is reserved", name)
	}
	if len(value) > maxCustomHeaderValueLength {
		return fmt.Errorf("custom Vault header %q exceeds the %d byte value limit", name, maxCustomHeaderValueLength)
	}
	for _, character := range value {
		if character == '\r' || character == '\n' || character == 0x7f || (character < 0x20 && character != '\t') {
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
		headers.Set(name, value)
	}
	sort.Strings(names)
	manifest, err := json.Marshal(names)
	if err != nil {
		return err
	}
	headers.Set(customHeadersManifestHeader, base64.RawURLEncoding.EncodeToString(manifest))
	return nil
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
