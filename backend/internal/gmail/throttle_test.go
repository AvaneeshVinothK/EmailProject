package gmail

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestThrottledTransport_RetriesRateLimitExceeded(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"error":{"code":403,"errors":[{"reason":"rateLimitExceeded"}]}}`)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()

	client := &http.Client{Transport: newThrottledTransport(http.DefaultTransport)}
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Get() returned error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status code = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if calls != 2 {
		t.Fatalf("request count = %d, want 2", calls)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll() error: %v", err)
	}
	if string(body) != "ok" {
		t.Fatalf("body = %q, want %q", string(body), "ok")
	}
}

func TestThrottledTransport_DoesNotRetryDailyLimitExceeded(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":{"code":403,"errors":[{"reason":"dailyLimitExceeded"}]}}`)
	}))
	defer server.Close()

	client := &http.Client{Transport: newThrottledTransport(http.DefaultTransport)}
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Get() returned error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status code = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
	if calls != 1 {
		t.Fatalf("request count = %d, want 1", calls)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll() error: %v", err)
	}
	if !strings.Contains(string(body), "dailyLimitExceeded") {
		t.Fatalf("body = %q, want dailyLimitExceeded text", string(body))
	}
}
