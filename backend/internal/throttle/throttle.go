package throttle

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

// Classify inspects an HTTP response and body, reporting whether the request should be retried and how long to wait before retrying.
type Classify func(resp *http.Response, body []byte) (wait time.Duration, retryable bool)

// Transport wraps an http.RoundTripper with per-client rate limiting and retry logic.
type Transport struct {
	base           http.RoundTripper
	limiter        *rate.Limiter
	maxRetries     int
	initialBackoff time.Duration
	maxBackoff     time.Duration
	classify       Classify
}

// New creates a Transport configured with its own limiter and retry behavior.
func New(base http.RoundTripper, requestsPerSecond float64, burst int, classify Classify) *Transport {
	if base == nil {
		base = http.DefaultTransport
	}
	if classify == nil {
		classify = func(resp *http.Response, body []byte) (time.Duration, bool) {
			return 0, false
		}
	}
	return &Transport{
		base:           base,
		limiter:        rate.NewLimiter(rate.Limit(requestsPerSecond), burst),
		maxRetries:     5,
		initialBackoff: 5 * time.Second,
		maxBackoff:     60 * time.Second,
		classify:       classify,
	}
}

func (t *Transport) roundTripOnce(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, err
		}
		clone.Body = body
	}
	return t.base.RoundTrip(clone)
}

// RoundTrip performs rate-limited HTTP calls with retry-on-transient-error support.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, http.ErrUseLastResponse
	}
	if req.Body != nil && req.Body != http.NoBody && req.GetBody == nil {
		if err := t.limiter.Wait(req.Context()); err != nil {
			return nil, err
		}
		return t.base.RoundTrip(req)
	}

	backoff := t.initialBackoff
	for attempt := 0; attempt <= t.maxRetries; attempt++ {
		if err := t.limiter.Wait(req.Context()); err != nil {
			return nil, err
		}

		resp, err := t.roundTripOnce(req)
		if err != nil {
			return nil, err
		}

		var body []byte
		var readErr error
		if resp.Body != nil {
			body, readErr = io.ReadAll(resp.Body)
			resp.Body.Close()
			resp.Body = io.NopCloser(bytes.NewReader(body))
		} else {
			body = nil
		}
		if readErr != nil {
			return resp, nil
		}

		wait, retryable := t.classify(resp, body)
		if !retryable || attempt == t.maxRetries {
			return resp, nil
		}

		sleepFor := wait
		if sleepFor <= 0 {
			sleepFor = backoff
			if backoff < t.maxBackoff {
				backoff *= 2
				if backoff > t.maxBackoff {
					backoff = t.maxBackoff
				}
			}
		}

		log.Printf("throttle: attempt=%d status=%d next_retry_sleep=%s", attempt+1, resp.StatusCode, sleepFor)
		select {
		case <-time.After(sleepFor):
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
	}

	return nil, fmt.Errorf("throttle: retries exhausted")
}
