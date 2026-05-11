package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"
)

func main() {
	url := env("HEALTHCHECK_URL", "http://127.0.0.1:8080/readyz")
	timeout, err := time.ParseDuration(env("HEALTHCHECK_TIMEOUT", "2s"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid HEALTHCHECK_TIMEOUT: %v\n", err)
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid HEALTHCHECK_URL: %v\n", err)
		os.Exit(2)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck request failed: %v\n", err)
		os.Exit(1)
	}
	defer res.Body.Close()

	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		fmt.Fprintf(os.Stderr, "healthcheck returned status %d\n", res.StatusCode)
		os.Exit(1)
	}
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
