package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

func checkAgentHealth(ctx context.Context, port string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+port+"/health", nil)
	if err != nil {
		return fmt.Errorf("creating health request: %w", err)
	}
	client := &http.Client{
		Timeout:   5 * time.Second,
		Transport: &http.Transport{DisableKeepAlives: true},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	defer client.CloseIdleConnections()
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("requesting Agent health: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Agent health returned HTTP %d", response.StatusCode)
	}
	var health struct {
		HTTPStatusCode   int    `json:"httpStatusCode"`
		EntityStatusCode string `json:"entityStatusCode"`
		Metadata         struct {
			ApplicationName string `json:"applicationName"`
		} `json:"metadata"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64*1024)).Decode(&health); err != nil {
		return fmt.Errorf("decoding Agent health: %w", err)
	}
	if health.HTTPStatusCode != http.StatusOK || health.EntityStatusCode != "healthy" || health.Metadata.ApplicationName != "agent" {
		return fmt.Errorf("Agent health returned an unexpected response")
	}
	return nil
}
