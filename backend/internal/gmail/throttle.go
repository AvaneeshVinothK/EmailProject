package gmail

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"google.golang.org/api/googleapi"
)

var ErrRateLimited = errors.New("gmail rate limit exceeded")

func gmailClassify(resp *http.Response, body []byte) (time.Duration, bool) {
	if resp == nil {
		return 0, false
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return 0, true
	}
	if resp.StatusCode != http.StatusForbidden {
		return 0, false
	}

	lowerBody := strings.ToLower(string(body))
	if strings.Contains(lowerBody, "ratelimitexceeded") {
		return 0, true
	}
	return 0, false
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
