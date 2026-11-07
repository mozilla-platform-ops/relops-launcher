package launcher

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type releaseCooldown struct {
	Until  time.Time `json:"until"`
	Reason string    `json:"reason"`
}

func releaseResponseError(url string, resp *http.Response, now time.Time) (error, time.Time) {
	var body struct {
		Message string `json:"message"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&body)
	message := strings.Join(strings.Fields(body.Message), " ")
	if len(message) > 512 {
		message = message[:512]
	}
	limited := resp.StatusCode == http.StatusTooManyRequests ||
		(resp.StatusCode == http.StatusForbidden && (resp.Header.Get("X-RateLimit-Remaining") == "0" ||
			resp.Header.Get("Retry-After") != "" || strings.Contains(strings.ToLower(message), "rate limit")))
	var until time.Time
	if limited {
		if reset, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil && resp.Header.Get("X-RateLimit-Remaining") == "0" {
			until = time.Unix(reset, 0)
		}
		if retry := resp.Header.Get("Retry-After"); retry != "" {
			var retryAt time.Time
			if seconds, err := strconv.ParseInt(retry, 10, 32); err == nil && seconds >= 0 {
				retryAt = now.Add(time.Duration(seconds) * time.Second)
			} else if date, err := http.ParseTime(retry); err == nil {
				retryAt = date
			}
			if retryAt.After(until) {
				until = retryAt
			}
		}
		if !until.After(now) {
			until = now.Add(time.Minute)
		}
		if message == "" {
			message = "GitHub rate limit exceeded"
		}
		message += "; retry after " + until.UTC().Format(time.RFC3339) + "; configure GH_TOKEN or GITHUB_TOKEN for authenticated API checks"
	} else if message == "" {
		message = "check release availability and access"
	}
	return fmt.Errorf("fetch %s: HTTP %d: %s", url, resp.StatusCode, message), until
}
