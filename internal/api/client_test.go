package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"lyftacli/internal/errs"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestGetAddsBearerTokenAndQuery(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if got := request.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("Authorization = %q", got)
		}
		if got := request.URL.Query().Get("limit"); got != "10" {
			t.Errorf("limit = %q", got)
		}
		return jsonResponse(http.StatusOK, `{"status":true}`), nil
	})
	client := newTestClient(t, Config{Token: "secret", BaseURL: "https://my.lyfta.app", transport: transport})
	result, err := client.Get(context.Background(), "/ok", url.Values{"limit": {"10"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.(map[string]any)["status"] != true {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestPostSendsJSON(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.Header.Get("Content-Type") != "application/json" {
			t.Errorf("method/content-type = %s/%s", request.Method, request.Header.Get("Content-Type"))
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["title"] != "Push day" {
			t.Errorf("body = %#v", body)
		}
		return jsonResponse(http.StatusOK, `{"status":true}`), nil
	})
	client := newTestClient(t, Config{Token: "secret", BaseURL: "https://my.lyfta.app", transport: transport})
	if _, err := client.Post(context.Background(), "/write", map[string]any{"title": "Push day"}); err != nil {
		t.Fatal(err)
	}
}

func TestAPIErrorPreservesDetails(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		response := jsonResponse(http.StatusUnauthorized, `{"status":false,"message":"Invalid API key"}`)
		response.Header.Set("X-Request-ID", "req-1")
		return response, nil
	})
	client := newTestClient(t, Config{Token: "secret", BaseURL: "https://my.lyfta.app", transport: transport})
	_, err := client.Get(context.Background(), "/error", nil)
	var apiErr *errs.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %T %v", err, err)
	}
	if apiErr.StatusCode != 401 || apiErr.Message != "Invalid API key" || apiErr.RequestID != "req-1" {
		t.Fatalf("API error = %#v", apiErr)
	}
}

func TestStatusFalseOnSuccessIsAPIError(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"status":false,"message":"collectionId is required"}`), nil
	})
	client := newTestClient(t, Config{Token: "secret", BaseURL: "https://my.lyfta.app", transport: transport})
	_, err := client.Post(context.Background(), "/templates", map[string]any{})
	var apiErr *errs.Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 200 || apiErr.Message != "collectionId is required" {
		t.Fatalf("error = %#v", err)
	}
}

func TestPagination(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		page := request.URL.Query().Get("page")
		body, _ := json.Marshal(map[string]any{
			"status": true, "total_pages": 2, "current_page": json.Number(page),
			"workouts": []any{map[string]any{"id": json.Number(page)}},
		})
		return jsonResponse(http.StatusOK, string(body)), nil
	})
	client := newTestClient(t, Config{Token: "secret", BaseURL: "https://my.lyfta.app", transport: transport})
	result, err := client.PaginatePages(context.Background(), "/pages", "workouts", url.Values{"page": {"1"}}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if result["pages_fetched"] != 2 || len(result["workouts"].([]any)) != 2 || result["truncated"] != false {
		t.Fatalf("result = %#v", result)
	}
}

func TestOffsetPagination(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		offset := request.URL.Query().Get("offset")
		body, _ := json.Marshal(map[string]any{"status": true, "data": map[string]any{
			"results":    []any{map[string]any{"offset": offset}},
			"pagination": map[string]any{"hasMore": offset == "0"},
		}})
		return jsonResponse(http.StatusOK, string(body)), nil
	})
	client := newTestClient(t, Config{Token: "secret", BaseURL: "https://my.lyfta.app", transport: transport})
	result, err := client.PaginateOffsets(context.Background(), "/offsets", url.Values{"offset": {"0"}}, 10)
	if err != nil {
		t.Fatal(err)
	}
	data := result["data"].(map[string]any)
	if len(data["results"].([]any)) != 2 || data["pagination"].(map[string]any)["pages_fetched"] != 2 {
		t.Fatalf("result = %#v", result)
	}
}

func TestRejectsUnsafeURLs(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, Config{Token: "secret", BaseURL: "https://my.lyfta.app"})
	for _, target := range []string{"https://example.com/api", "mailto:attacker@example.com"} {
		if _, err := client.Get(context.Background(), target, nil); err == nil {
			t.Errorf("expected %q to fail", target)
		}
	}
	if _, err := New(Config{Token: "secret", BaseURL: "http://example.com"}); err == nil {
		t.Error("expected non-loopback HTTP to fail")
	}
	if _, err := New(Config{Token: "secret", BaseURL: "http://127.0.0.1:8080"}); err != nil {
		t.Fatalf("loopback HTTP: %v", err)
	}
}

func TestPostTransportFailureIsNotRetried(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	client := newTestClient(t, Config{
		Token: "secret", BaseURL: "https://my.lyfta.app", MaxRetries: 2,
		transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			calls.Add(1)
			return nil, errors.New("connection lost")
		}),
		sleep: noSleep,
	})
	if _, err := client.Post(context.Background(), "/collections", map[string]any{}); err == nil {
		t.Fatal("expected transport error")
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d", calls.Load())
	}
}

func TestPostServerFailureIsNotRetried(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	client := newTestClient(t, Config{
		Token: "secret", BaseURL: "https://my.lyfta.app", MaxRetries: 2,
		transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			calls.Add(1)
			return jsonResponse(http.StatusServiceUnavailable, `{"message":"try later"}`), nil
		}),
		sleep: noSleep,
	})
	if _, err := client.Post(context.Background(), "/collections", map[string]any{}); err == nil {
		t.Fatal("expected API error")
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d", calls.Load())
	}
}

func TestGetTransportFailureIsRetried(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	client := newTestClient(t, Config{
		Token: "secret", BaseURL: "https://my.lyfta.app", MaxRetries: 2,
		transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			if calls.Add(1) == 1 {
				return nil, errors.New("connection lost")
			}
			return jsonResponse(http.StatusOK, `{"status":true}`), nil
		}),
		sleep: noSleep,
	})
	if _, err := client.Get(context.Background(), "/workouts", nil); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls = %d", calls.Load())
	}
}

func TestRetryAfterSupportsSecondsAndDate(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	if delay, ok := retryDelay("12", now); !ok || delay != 12*time.Second {
		t.Fatalf("seconds = %v, %v", delay, ok)
	}
	date := now.Add(30 * time.Second).Format(http.TimeFormat)
	if delay, ok := retryDelay(date, now); !ok || delay != 30*time.Second {
		t.Fatalf("date = %v, %v", delay, ok)
	}
}

func TestCrossOriginRedirectIsRejected(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, Config{Token: "secret", BaseURL: "https://my.lyfta.app"})
	redirect, _ := http.NewRequest(http.MethodGet, "https://example.com/collect", nil)
	previous, _ := http.NewRequest(http.MethodGet, "https://my.lyfta.app/redirect", nil)
	previous.Response = &http.Response{StatusCode: http.StatusFound}
	err := client.http.CheckRedirect(redirect, []*http.Request{previous})
	var apiErr *errs.Error
	if !errors.As(err, &apiErr) || apiErr.Message != "Refusing to forward credentials across origins" {
		t.Fatalf("error = %#v", err)
	}
}

func TestResponseBodyLimit(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, Config{
		Token: "secret", BaseURL: "https://my.lyfta.app",
		transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"text/plain"}},
				Body:       io.NopCloser(strings.NewReader(strings.Repeat("x", maxResponseBodySize+1))),
			}, nil
		}),
	})
	_, err := client.Get(context.Background(), "/large", nil)
	var transportErr *errs.Error
	if !errors.As(err, &transportErr) || transportErr.Kind != errs.KindTransport {
		t.Fatalf("error = %#v", err)
	}
}

func TestRetryWaitHonorsCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	client := newTestClient(t, Config{
		Token: "secret", BaseURL: "https://my.lyfta.app", MaxRetries: 1,
		transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("connection lost")
		}),
		sleep: func(context.Context, time.Duration) error {
			cancel()
			return ctx.Err()
		},
	})
	_, err := client.Get(ctx, "/workouts", nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %#v", err)
	}
}

func newTestClient(t *testing.T, config Config) *Client {
	t.Helper()
	if config.Timeout == 0 {
		config.Timeout = time.Second
	}
	client, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status, Status: http.StatusText(status),
		Header: http.Header{"Content-Type": {"application/json"}},
		Body:   io.NopCloser(strings.NewReader(body)),
	}
}

func noSleep(context.Context, time.Duration) error { return nil }
