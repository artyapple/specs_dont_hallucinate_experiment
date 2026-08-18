package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidAuthority(t *testing.T) {
	for _, authority := range []string{"openrouter.ai", "https://openrouter.ai:443", "127.0.0.1:443", "openrouter.ai@evil:443"} {
		if validAuthority(authority) {
			t.Errorf("validAuthority(%q) = true", authority)
		}
	}
	if !validAuthority("openrouter.ai:443") {
		t.Fatal("exact DNS authority must be valid")
	}
}

func TestHealthHandler(t *testing.T) {
	for _, test := range []struct {
		method, path string
		status       int
	}{
		{http.MethodGet, "/healthz", http.StatusOK},
		{http.MethodPost, "/healthz", http.StatusNotFound},
		{http.MethodGet, "/other", http.StatusNotFound},
	} {
		request := httptest.NewRequest(test.method, test.path, nil)
		response := httptest.NewRecorder()
		healthHandler(response, request)
		if response.Code != test.status {
			t.Errorf("%s %s = %d, want %d", test.method, test.path, response.Code, test.status)
		}
	}
}
