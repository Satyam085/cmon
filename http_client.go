package main

import (
	"crypto/tls"
	"net/http"
	"time"
)

// httpClient is a package-level reusable HTTP client with proper timeouts
// and connection pooling. Initialized once and reused across all requests.
var httpClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{
			// TODO: Fix certificate validation instead of skipping
			// This is currently needed for the DGVCL complaint API
			InsecureSkipVerify: true,
		},
		MaxIdleConns:        10,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  false,
		MaxIdleConnsPerHost: 5,
	},
}

// GetHTTPClient returns the shared HTTP client instance
func GetHTTPClient() *http.Client {
	return httpClient
}
