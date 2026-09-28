package gmail

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/time/rate"
	"google.golang.org/api/googleapi"
)

var ErrRateLimited = errors.New("gmail rate limit exceeded")

const (
	requestsPerSecond = 10
	burst             = 1
	maxRetries        = 5
)

type throttledTransport struct {
	base    http.RoundTripper
	limiter *rate.Limiter
}

func newThrottledTransport(base http.RoundTripper) *throttledTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	return &throttledTransport{
		base:    base,
		limiter: rate.NewLimiter(rate.Limit(requestsPerSecond), burst),
	}
}

func (t *throttledTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var lastResp *http.Response
	backoff := 5 * time.Second
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := t.limiter.Wait(req.Context()); err != nil {
			return nil, err
		}
		resp, err := t.base.RoundTrip(req)
		if err != nil {
			return nil, err
		}
		lastResp = resp
		if !isRateLimitResponse(resp) {
			return resp, nil
		}
		if attempt == maxRetries {
			return resp, nil
		}
		select {
		case <-time.After(backoff):
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
		if backoff < 60*time.Second {
			backoff *= 2
			if backoff > 60*time.Second {
				backoff = 60 * time.Second
			}
		}
	}
	return lastResp, nil
}

func isRateLimitResponse(resp *http.Response) bool {
	if resp == nil {
		return false
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if resp.StatusCode != http.StatusForbidden || resp.Body == nil {
		return false
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		resp.Body = io.NopCloser(bytes.NewReader(nil))
		return false
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	lowerBody := strings.ToLower(string(body))
	return strings.Contains(lowerBody, "ratelimitexceeded")
}

func isRateLimitErr(err error) bool {
	var gerr *googleapi.Error
	if !errors.As(err, &gerr) {
		return false
	}
	if gerr.Code == http.StatusTooManyRequests {
		return true
	}
	if gerr.Code != http.StatusForbidden {
		return false
	}
	for _, apiErr := range gerr.Errors {
		reason := strings.ToLower(apiErr.Reason)
		if strings.Contains(reason, "ratelimitexceeded") {
			return true
		}
	}
	return false
}

func wrapAPIError(what string, err error) error {
	if isRateLimitErr(err) {
		return fmt.Errorf("%s: %w: %w", what, ErrRateLimited, err)
	}
	return fmt.Errorf("%s: %w", what, err)
}
