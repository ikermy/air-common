// Package httpclient provides a small HTTP client with retries for idempotent
// requests and a consecutive-failure circuit breaker.
package httpclient

import (
	"errors"
	"io"
	"net/http"
	"sync"
	"time"
)

// ErrCircuitOpen is returned when the circuit breaker is open.
var ErrCircuitOpen = errors.New("httpclient: circuit breaker is open")

// Options configures the client. Zero values are replaced with defaults.
type Options struct {
	Timeout          time.Duration
	Retries          int
	Backoff          time.Duration
	FailureThreshold int
	OpenFor          time.Duration
}

func (o Options) withDefaults() Options {
	if o.Timeout <= 0 {
		o.Timeout = 30 * time.Second
	}
	if o.Retries < 0 {
		o.Retries = 0
	}
	if o.Backoff <= 0 {
		o.Backoff = 100 * time.Millisecond
	}
	if o.FailureThreshold <= 0 {
		o.FailureThreshold = 5
	}
	if o.OpenFor <= 0 {
		o.OpenFor = 10 * time.Second
	}
	return o
}

// Client wraps http.Client.
type Client struct {
	base *http.Client
	opts Options

	mu        sync.Mutex
	failures  int
	openUntil time.Time
}

// New creates a client.
func New(opts Options) *Client {
	opts = opts.withDefaults()
	return &Client{
		base: &http.Client{Timeout: opts.Timeout},
		opts: opts,
	}
}

// Do performs a request without retries.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	return c.DoWithPolicy(req, false)
}

// DoWithPolicy performs a request. When retryable is true, 5xx/429 responses
// and network errors are retried (requires req.GetBody for replay).
func (c *Client) DoWithPolicy(req *http.Request, retryable bool) (*http.Response, error) {
	if !c.allow() {
		return nil, ErrCircuitOpen
	}

	attempts := 0
	for {
		attemptReq, err := cloneForAttempt(req)
		if err != nil {
			c.recordFailure()
			return nil, err
		}

		resp, err := c.base.Do(attemptReq)
		retryStatus := resp != nil && (resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests)

		if err == nil && !retryStatus {
			c.recordSuccess()
			return resp, nil
		}
		if resp != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}

		attempts++
		if !retryable || attempts > c.opts.Retries || req.GetBody == nil {
			c.recordFailure()
			return resp, err
		}
		time.Sleep(c.opts.Backoff * time.Duration(1<<min(attempts-1, 3)))
	}
}

func cloneForAttempt(req *http.Request) (*http.Request, error) {
	clone := req.Clone(req.Context())
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, err
		}
		clone.Body = body
	}
	return clone, nil
}

func (c *Client) allow() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.openUntil.IsZero() {
		return true
	}
	if time.Now().After(c.openUntil) {
		// Half-open: allow one probe request; a failure will reopen the circuit.
		c.openUntil = time.Time{}
		return true
	}
	return false
}

func (c *Client) recordSuccess() {
	c.mu.Lock()
	c.failures = 0
	c.openUntil = time.Time{}
	c.mu.Unlock()
}

func (c *Client) recordFailure() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failures++
	if c.failures >= c.opts.FailureThreshold {
		c.openUntil = time.Now().Add(c.opts.OpenFor)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
