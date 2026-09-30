package gmail

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AvaneeshVinothK/EmailProject/internal/throttle"
)

func TestGmailClassify_RetriesRateLimitExceeded(t *testing.T) {
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

	client := &http.Client{Transport: throttle.New(http.DefaultTransport, 10, 1, gmailClassify)}
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

func TestGmailClassify_DoesNotRetryDailyLimitExceeded(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":{"code":403,"errors":[{"reason":"dailyLimitExceeded"}]}}`)
	}))
	defer server.Close()

	client := &http.Client{Transport: throttle.New(http.DefaultTransport, 10, 1, gmailClassify)}
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

func TestGmailClassify_UnitCases(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     string
		wantWait time.Duration
		wantOK   bool
	}{
		{name: "429", status: http.StatusTooManyRequests, body: `RESOURCE_EXHAUSTED`, wantWait: 0, wantOK: true},
		{name: "403 rate limit", status: http.StatusForbidden, body: `rateLimitExceeded`, wantWait: 0, wantOK: true},
		{name: "403 daily limit", status: http.StatusForbidden, body: `dailyLimitExceeded`, wantWait: 0, wantOK: false},
		{name: "403 unrelated", status: http.StatusForbidden, body: `other issue`, wantWait: 0, wantOK: false},
		{name: "200", status: http.StatusOK, body: `ok`, wantWait: 0, wantOK: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wait, ok := gmailClassify(&http.Response{StatusCode: tc.status}, []byte(tc.body))
			if ok != tc.wantOK {
				t.Fatalf("gmailClassify ok = %v, want %v", ok, tc.wantOK)
			}
			if wait != tc.wantWait {
				t.Fatalf("gmailClassify wait = %v, want %v", wait, tc.wantWait)
			}
		})
	}
}
