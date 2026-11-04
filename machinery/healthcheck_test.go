package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCheckAgentHealth(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		wantOK bool
	}{
		{"healthy", 200, `{"httpStatusCode":200,"entityStatusCode":"healthy","metadata":{"applicationName":"agent"}}`, true},
		{"unhealthy status", 503, `{}`, false},
		{"redirect", 302, `{}`, false},
		{"invalid JSON", 200, `not JSON`, false},
		{"wrong service", 200, `{"httpStatusCode":200,"entityStatusCode":"healthy","metadata":{"applicationName":"other"}}`, false},
		{"unhealthy body", 200, `{"httpStatusCode":200,"entityStatusCode":"unhealthy","metadata":{"applicationName":"agent"}}`, false},
	}
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("NO_PROXY", "")
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/health" {
					t.Errorf("path = %q, want /health", r.URL.Path)
				}
				w.Header().Set("Location", "http://127.0.0.1:1")
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			_, port, err := net.SplitHostPort(server.Listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			err = checkAgentHealth(context.Background(), port)
			if (err == nil) != test.wantOK {
				t.Fatalf("checkAgentHealth() = %v, wantOK %t", err, test.wantOK)
			}
		})
	}
}

func TestCheckAgentHealthTimesOut(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	_, port, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	if err := checkAgentHealth(ctx, port); err == nil {
		t.Fatal("healthcheck succeeded despite its deadline")
	}
}
