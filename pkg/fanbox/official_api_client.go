package fanbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/http/httputil"
	"reflect"
	"sync"

	"github.com/hashicorp/go-retryablehttp"
	"golang.org/x/time/rate"
)

type OfficialAPIClient struct {
	HTTPClient *retryablehttp.Client
	Cookie     string
	UserAgent  string
	rateLimiter *rate.Limiter
	rateLimiterMu sync.RWMutex
	onJSONReceived JSONReceivedCallback
}

// JSONReceivedCallback is called when JSON is received from the API
type JSONReceivedCallback func(url string, jsonData []byte)

// SetRateLimit sets the rate limit for API requests
func (c *OfficialAPIClient) SetRateLimit(requestsPerSecond float64) {
	c.rateLimiterMu.Lock()
	defer c.rateLimiterMu.Unlock()
	c.rateLimiter = rate.NewLimiter(rate.Limit(requestsPerSecond), 1)
}

// SetJSONCallback sets the callback function to be called when JSON is received
func (c *OfficialAPIClient) SetJSONSetCallback(callback JSONReceivedCallback) {
	c.rateLimiterMu.Lock()
	defer c.rateLimiterMu.Unlock()
	c.onJSONReceived = callback
}

func (c *OfficialAPIClient) Request(ctx context.Context, method string, url string) (*http.Response, error) {
	// Wait for rate limiter if configured
	c.rateLimiterMu.RLock()
	limiter := c.rateLimiter
	c.rateLimiterMu.RUnlock()
	
	if limiter != nil {
		if err := limiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limit wait: %w", err)
		}
	}

	req, err := retryablehttp.NewRequest(method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("http request building error: %w", err)
	}

	req = req.WithContext(ctx)
	req.Header.Set("Cookie", c.Cookie)
	req.Header.Set("Origin", "https://www.fanbox.cc") // If Origin header is not set, FANBOX returns HTTP 400 error.
	req.Header.Set("Referer", "https://www.fanbox.cc/")
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Encoding", "gzip")

	return c.HTTPClient.Do(req)
}

func (c *OfficialAPIClient) RequestAndUnwrapJSON(ctx context.Context, method string, url string, v interface{}) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("v should be a pointer")
	}

	resp, err := c.Request(ctx, method, url)
	if err != nil {
		return fmt.Errorf("http error: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != 200 {
		return fmt.Errorf("status is %s", resp.Status)
	}

	// Read the entire response body first
	jsonData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	// Call the JSON callback if configured
	c.rateLimiterMu.RLock()
	callback := c.onJSONReceived
	c.rateLimiterMu.RUnlock()
	if callback != nil {
		callback(url, jsonData)
	}

	// Decode JSON
	if err = json.Unmarshal(jsonData, v); err != nil {
		if dump, dumpErr := httputil.DumpResponse(resp, false); dumpErr == nil {
			slog.Debug("Response dump", "dump", string(dump))
		}
		return fmt.Errorf("json decoding error: %w", err)
	}
	return nil
}

var ErrFailedToThumbnailing = fmt.Errorf("failed to thumbnailing")

// fanbox returns HTTP 500 error and response body is "failed to thumbnailing"
// when the image is not available (e.g. too large).
func IsFailedToThumbnailingErr(resp *http.Response) (bool, error) {
	if resp.StatusCode != 500 {
		return false, nil
	}
	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		return false, fmt.Errorf("parse content type: %w", err)
	}
	if mediaType != "text/plain" {
		return false, nil
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("read response body: %w", err)
	}
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(b))

	if string(b) == "failed to thumbnailing" {
		return true, nil
	}
	return false, nil
}
