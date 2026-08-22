// Package httpclient provides the one HTTP client every scraping tier uses.
//
// Two behaviours are shared by all vendors and are easy to get subtly wrong by
// hand, so both come from established packages: retry with jittered
// exponential backoff (hashicorp/go-retryablehttp) and a token-bucket rate
// limit that keeps request spacing polite (golang.org/x/time/rate).
package httpclient

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"golang.org/x/time/rate"
)

// browserUserAgent identifies the scraper as an ordinary browser. Several of
// the tracked storefronts sit behind protection that rejects the default Go
// user agent outright.
const browserUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) " +
	"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

// maximumResponseBytes caps a single response. Catalogue JSON pages run to a
// few megabytes; anything far larger means we have been handed something
// unexpected and should fail rather than buffer it.
const maximumResponseBytes = 64 << 20

// Client is a polite, retrying HTTP client scoped to one vendor.
type Client struct {
	retryableClient *retryablehttp.Client
	rateLimiter     *rate.Limiter
	logger          *slog.Logger
}

// New builds a client that spaces requests by requestDelay and retries
// transient failures. A zero or negative requestDelay disables rate limiting.
func New(requestDelay time.Duration, logger *slog.Logger) *Client {
	retryableClient := retryablehttp.NewClient()
	retryableClient.RetryMax = 3
	retryableClient.RetryWaitMin = 2 * time.Second
	retryableClient.RetryWaitMax = 30 * time.Second
	retryableClient.HTTPClient.Timeout = 60 * time.Second
	// retryablehttp logs every attempt at its own level; route that through
	// our logger instead of stderr noise.
	retryableClient.Logger = nil

	var rateLimiter *rate.Limiter
	if requestDelay > 0 {
		// One request per delay period, no burst: strict spacing rather than
		// a burst followed by a stall.
		rateLimiter = rate.NewLimiter(rate.Every(requestDelay), 1)
	}

	return &Client{
		retryableClient: retryableClient,
		rateLimiter:     rateLimiter,
		logger:          logger,
	}
}

// GetBytes fetches a URL and returns the whole response body.
func (client *Client) GetBytes(ctx context.Context, requestURL string) ([]byte, error) {
	if client.rateLimiter != nil {
		if err := client.rateLimiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("waiting for rate limiter before %s: %w", requestURL, err)
		}
	}

	request, err := retryablehttp.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building request for %s: %w", requestURL, err)
	}
	request.Header.Set("User-Agent", browserUserAgent)
	request.Header.Set("Accept-Language", "en-IN,en;q=0.9")

	response, err := client.retryableClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", requestURL, err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching %s: unexpected status %d", requestURL, response.StatusCode)
	}

	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maximumResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("reading response from %s: %w", requestURL, err)
	}

	client.logger.Debug("fetched", "url", requestURL, "bytes", len(responseBody))
	return responseBody, nil
}

// NotFoundError reports a 404, which callers treat as "this product is gone"
// rather than as a scrape failure.
type NotFoundError struct {
	RequestURL string
}

func (err *NotFoundError) Error() string {
	return fmt.Sprintf("%s returned 404", err.RequestURL)
}
