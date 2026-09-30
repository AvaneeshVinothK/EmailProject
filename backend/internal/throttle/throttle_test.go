package throttle

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestTransport_RetriesRateLimitResponse(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `RESOURCE_EXHAUSTED ... retryDelay:1s`)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()

	transport := New(http.DefaultTransport, 1000, 1, func(resp *http.Response, body []byte) (time.Duration, bool) {
		if resp.StatusCode == http.StatusTooManyRequests {
			return time.Second, true
		}
		return 0, false
	})
	transport.initialBackoff = 10 * time.Millisecond
	transport.maxBackoff = 30 * time.Millisecond

	client := &http.Client{Transport: transport}
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

func TestTransport_NoRetryWhenClassifySaysNo(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `bad request`)
	}))
	defer server.Close()

	transport := New(http.DefaultTransport, 1000, 1, func(resp *http.Response, body []byte) (time.Duration, bool) {
		return 0, false
	})
	transport.initialBackoff = 10 * time.Millisecond
	transport.maxBackoff = 30 * time.Millisecond

	client := &http.Client{Transport: transport}
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
}

func TestTransport_RetriesReplayRequestBody(t *testing.T) {
	const payload = `{"message":"hello world","count":3}`

	var (
		mu     sync.Mutex
		bodies []string
		calls  int
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		mu.Lock()
		bodies = append(bodies, string(body))
		mu.Unlock()

		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, "retry")
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()

	transport := New(http.DefaultTransport, 1000, 10, func(resp *http.Response, body []byte) (time.Duration, bool) {
		if resp.StatusCode == http.StatusServiceUnavailable {
			return 5 * time.Millisecond, true
		}
		return 0, false
	})
	transport.initialBackoff = 5 * time.Millisecond
	transport.maxBackoff = 20 * time.Millisecond

	client := &http.Client{Transport: transport}
	req, err := http.NewRequest(http.MethodPost, server.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest() returned error: %v", err)
	}
	req.Body = io.NopCloser(strings.NewReader(payload))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(payload)), nil
	}
	req.ContentLength = int64(len(payload))
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do() returned error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status code = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if calls != 2 {
		t.Fatalf("request count = %d, want 2", calls)
	}

	if len(bodies) != 2 {
		t.Fatalf("body count = %d, want 2", len(bodies))
	}
	if bodies[0] != payload || bodies[1] != payload {
		t.Fatalf("bodies = %q, %q; want both %q", bodies[0], bodies[1], payload)
	}
}
