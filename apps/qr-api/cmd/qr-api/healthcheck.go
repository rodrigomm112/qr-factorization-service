package main

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

// healthcheckTimeout bounds the container probe, under the 3s compose allows it.
const healthcheckTimeout = 2 * time.Second

// healthcheck is the "healthcheck" subcommand: the distroless image has no shell, no curl.
func healthcheck(getenv func(string) string, stderr io.Writer) int {
	port := getenv("PORT")
	if port == "" {
		port = "8080"
	}

	client := &http.Client{Timeout: healthcheckTimeout}
	resp, err := client.Get("http://127.0.0.1:" + port + "/health/live") //nolint:noctx // the client timeout is the deadline
	if err != nil {
		fmt.Fprintf(stderr, "healthcheck: %v\n", err)
		return 1
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(stderr, "healthcheck: status %d\n", resp.StatusCode)
		return 1
	}
	return 0
}
