package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/samuelfarkas/lyftacli/internal/errs"
	"github.com/samuelfarkas/lyftacli/internal/version"
)

const (
	DefaultBaseURL      = "https://my.lyfta.app"
	DefaultTimeout      = 30 * time.Second
	DefaultMaxRetries   = 2
	maxRetryDelay       = 60 * time.Second
	maxResponseBodySize = 16 << 20
)

type Config struct {
	Token      string
	BaseURL    string
	Timeout    time.Duration
	MaxRetries int
	transport  http.RoundTripper
	sleep      func(context.Context, time.Duration) error
}

type Client struct {
	token      string
	base       *url.URL
	origin     string
	maxRetries int
	http       *http.Client
	sleep      func(context.Context, time.Duration) error
}

func New(config Config) (*Client, error) {
	token := strings.TrimSpace(config.Token)
	if token == "" {
		return nil, errs.New(errs.KindConfiguration, "No API token configured. Set LYFTA_API_TOKEN or use --token-file.")
	}
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	base, err := url.Parse(baseURL)
	if err != nil || base.Scheme == "" || base.Host == "" || (base.Scheme != "http" && base.Scheme != "https") {
		return nil, errs.New(errs.KindConfiguration, "Base URL must be an absolute HTTP(S) URL")
	}
	if base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return nil, errs.New(errs.KindConfiguration, "Base URL cannot contain credentials, a query, or a fragment")
	}
	if base.Path != "" && base.Path != "/" {
		return nil, errs.New(errs.KindConfiguration, "Base URL must be an origin without a path")
	}
	if base.Scheme != "https" && !isLoopback(base.Hostname()) {
		return nil, errs.New(errs.KindConfiguration, "Base URL must use HTTPS except on loopback")
	}
	if config.Timeout == 0 {
		config.Timeout = DefaultTimeout
	}
	if config.Timeout < 0 {
		return nil, errs.New(errs.KindConfiguration, "Timeout must be greater than zero")
	}
	if config.MaxRetries < 0 {
		return nil, errs.New(errs.KindConfiguration, "Max retries cannot be negative")
	}
	transport := config.transport
	if transport == nil {
		transport = http.DefaultTransport.(*http.Transport).Clone()
	}
	origin := canonicalOrigin(base)
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   config.Timeout,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errs.New(errs.KindAPI, "Stopped after 10 redirects")
			}
			if canonicalOrigin(request.URL) != origin {
				return &errs.Error{
					Kind:       errs.KindAPI,
					Message:    "Refusing to forward credentials across origins",
					StatusCode: redirectStatus(via),
					Details:    map[string]any{"location": request.URL.String()},
				}
			}
			return nil
		},
	}
	sleep := config.sleep
	if sleep == nil {
		sleep = sleepContext
	}
	return &Client{
		token: token, base: base, origin: origin, maxRetries: config.MaxRetries,
		http: httpClient, sleep: sleep,
	}, nil
}

func (c *Client) Get(ctx context.Context, path string, params url.Values) (any, error) {
	return c.Request(ctx, http.MethodGet, path, params, nil)
}

func (c *Client) Post(ctx context.Context, path string, body any) (any, error) {
	return c.Request(ctx, http.MethodPost, path, nil, body)
}

func (c *Client) Request(ctx context.Context, method, path string, params url.Values, body any) (any, error) {
	method = strings.ToUpper(method)
	target, err := c.buildURL(path, params)
	if err != nil {
		return nil, err
	}
	var encoded []byte
	if body != nil {
		encoded, err = json.Marshal(body)
		if err != nil {
			return nil, errs.Wrap(errs.KindInput, "Could not encode request body", err)
		}
	}

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		request, requestErr := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(encoded))
		if requestErr != nil {
			return nil, errs.Wrap(errs.KindValidation, "Could not build request", requestErr)
		}
		request.Header.Set("Accept", "application/json")
		request.Header.Set("Authorization", "Bearer "+c.token)
		request.Header.Set("User-Agent", "lyftacli/"+version.Value+" (+https://my.lyfta.app/community/api)")
		if body != nil {
			request.Header.Set("Content-Type", "application/json")
		}

		response, requestErr := c.http.Do(request)
		if requestErr != nil {
			var lyftaErr *errs.Error
			if errors.As(requestErr, &lyftaErr) {
				return nil, lyftaErr
			}
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if retryableMethod(method) && attempt < c.maxRetries {
				if err := c.wait(ctx, attempt, ""); err != nil {
					return nil, err
				}
				continue
			}
			return nil, errs.Wrap(errs.KindTransport, "Request failed", requestErr)
		}

		payload, readErr := readResponse(response)
		if readErr != nil {
			return nil, readErr
		}
		if shouldRetry(method, response.StatusCode, attempt, c.maxRetries) {
			if err := c.wait(ctx, attempt, response.Header.Get("Retry-After")); err != nil {
				return nil, err
			}
			continue
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return nil, &errs.Error{
				Kind: errs.KindAPI, Message: errorMessage(payload, response.Status),
				StatusCode: response.StatusCode, Details: payload,
				RequestID: response.Header.Get("X-Request-ID"),
			}
		}
		if object, ok := payload.(map[string]any); ok && object["status"] == false {
			return nil, &errs.Error{
				Kind: errs.KindAPI, Message: errorMessage(payload, "Lyfta API returned status=false"),
				StatusCode: response.StatusCode, Details: payload,
			}
		}
		return payload, nil
	}
	return nil, errs.New(errs.KindTransport, "Request failed after retries")
}

func (c *Client) buildURL(path string, params url.Values) (string, error) {
	target, err := url.Parse(path)
	if err != nil {
		return "", errs.New(errs.KindValidation, "Request URL is invalid")
	}
	if !target.IsAbs() {
		if target.Host != "" {
			return "", errs.New(errs.KindValidation, "Request path must be an HTTP(S) URL or an API path")
		}
		reference := &url.URL{Path: strings.TrimPrefix(target.Path, "/"), RawQuery: target.RawQuery}
		target = c.base.ResolveReference(reference)
	}
	if target.Scheme != "http" && target.Scheme != "https" {
		return "", errs.New(errs.KindValidation, "Request path must be an HTTP(S) URL or an API path")
	}
	if canonicalOrigin(target) != c.origin {
		return "", errs.New(errs.KindValidation, "Request URL must use the configured Lyfta origin")
	}
	query := target.Query()
	for key, values := range params {
		for _, value := range values {
			query.Add(key, value)
		}
	}
	target.RawQuery = query.Encode()
	return target.String(), nil
}

func (c *Client) wait(ctx context.Context, attempt int, retryAfter string) error {
	delay, ok := retryDelay(retryAfter, time.Now())
	if !ok {
		delay = min(500*time.Millisecond*time.Duration(1<<attempt)+time.Duration(rand.Float64()*200*float64(time.Millisecond)), 5*time.Second)
	}
	return c.sleep(ctx, min(delay, maxRetryDelay))
}

func retryDelay(value string, now time.Time) (time.Duration, bool) {
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseFloat(value, 64); err == nil {
		return max(0, time.Duration(seconds*float64(time.Second))), true
	}
	retryAt, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}
	return max(0, retryAt.Sub(now)), true
}

func readResponse(response *http.Response) (any, error) {
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBodySize+1))
	if err != nil {
		return nil, errs.Wrap(errs.KindTransport, "Could not read response", err)
	}
	if len(data) > maxResponseBodySize {
		return nil, errs.New(errs.KindTransport, "Response body exceeds 16 MiB safety limit")
	}
	if len(data) == 0 {
		return nil, nil
	}
	trimmed := bytes.TrimSpace(data)
	if strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "json") || (len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[')) {
		var value any
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err == nil {
			return value, nil
		}
	}
	return string(data), nil
}

func canonicalOrigin(value *url.URL) string {
	port := value.Port()
	if port == "" {
		if strings.EqualFold(value.Scheme, "https") {
			port = "443"
		} else {
			port = "80"
		}
	}
	return strings.ToLower(value.Scheme) + "://" + strings.ToLower(value.Hostname()) + ":" + port
}

func isLoopback(host string) bool {
	ip := net.ParseIP(host)
	return strings.EqualFold(host, "localhost") || (ip != nil && ip.IsLoopback())
}

func retryableMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodPut, http.MethodDelete:
		return true
	default:
		return false
	}
}

func shouldRetry(method string, status, attempt, maxRetries int) bool {
	if !retryableMethod(method) || attempt >= maxRetries {
		return false
	}
	switch status {
	case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func errorMessage(details any, fallback string) string {
	if object, ok := details.(map[string]any); ok {
		for _, key := range []string{"message", "error", "detail"} {
			if value, ok := object[key].(string); ok && value != "" {
				return value
			}
		}
	}
	if value, ok := details.(string); ok && strings.TrimSpace(value) != "" {
		value = strings.TrimSpace(value)
		if len(value) > 500 {
			value = value[:500]
		}
		return value
	}
	return fallback
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func redirectStatus(via []*http.Request) int {
	if len(via) == 0 || via[len(via)-1].Response == nil {
		return http.StatusFound
	}
	return via[len(via)-1].Response.StatusCode
}

func jsonNumberInt(value any) (int, bool) {
	switch number := value.(type) {
	case json.Number:
		parsed, err := strconv.Atoi(number.String())
		return parsed, err == nil
	case float64:
		return int(number), number == float64(int(number))
	case int:
		return number, true
	default:
		return 0, false
	}
}
