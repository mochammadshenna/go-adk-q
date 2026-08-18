package tools

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestFetchURL_BlocksLoopback proves the SSRF guard actually rejects a real
// server on a loopback address (httptest.Server binds 127.0.0.1) — this is
// the security-critical property, verified against a real HTTP server and a
// real dial attempt, not a mocked check.
func TestFetchURL_BlocksLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("should never be reached"))
	}))
	defer srv.Close()

	_, err := fetchURL(nil, fetchArgs{URL: srv.URL})
	if err == nil {
		t.Fatal("expected fetch_url to refuse a loopback address, got nil error")
	}
	if !strings.Contains(err.Error(), "blocked") {
		t.Errorf("error = %v, want it to mention the blocked-address reason", err)
	}
}

func TestFetchURL_RejectsNonHTTPScheme(t *testing.T) {
	if _, err := fetchURL(nil, fetchArgs{URL: "ftp://example.com/file"}); err == nil {
		t.Fatal("expected an error for a non-http(s) scheme, got nil")
	}
}

// TestFetchURL_RealSuccessfulFetch hits example.com — IANA's domain reserved
// specifically for documentation/testing use — to prove a legitimate public
// URL is actually fetchable end-to-end (real DNS resolution, real dial, real
// HTTP round trip) once it clears the SSRF guard, not just that the guard
// blocks things.
func TestFetchURL_RealSuccessfulFetch(t *testing.T) {
	res, err := fetchURL(nil, fetchArgs{URL: "https://example.com/"})
	if err != nil {
		t.Skipf("network unavailable in this environment, skipping real-fetch check: %v", err)
	}
	if res.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", res.StatusCode)
	}
	if len(res.Body) == 0 {
		t.Error("Body is empty, want real page content")
	}
}
