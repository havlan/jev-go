package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultBaseURL     = "https://api.typesafe.ai/v1/systemone"
	DefaultModel       = "jev-latest"
	DefaultTimeout     = 10 * time.Second
	DefaultMaxAttempts = 3
	maxResponseBytes   = 1024 * 1024
)

type Config struct {
	APIKey      string
	BaseURL     string
	Model       string
	HTTPClient  *http.Client
	MaxAttempts int
}

type Client struct {
	apiKey      string
	baseURL     string
	model       string
	httpClient  *http.Client
	maxAttempts int
}

func New(config Config) (*Client, error) {
	apiKey := strings.TrimSpace(config.APIKey)
	if apiKey == "" {
		return nil, errors.New("API key must not be empty")
	}
	baseURL := strings.TrimSpace(config.BaseURL)
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	model := strings.TrimSpace(config.Model)
	if model == "" {
		model = DefaultModel
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: DefaultTimeout}
	}
	maxAttempts := config.MaxAttempts
	if maxAttempts == 0 {
		maxAttempts = DefaultMaxAttempts
	}
	if maxAttempts < 1 {
		return nil, errors.New("max attempts must be at least one")
	}
	return &Client{apiKey, baseURL, model, httpClient, maxAttempts}, nil
}

func NewFromEnv() (*Client, error) {
	return New(Config{APIKey: os.Getenv("TYPESAFE_API_KEY")})
}

func (c *Client) SystemOne(ctx context.Context, state any, questions Questions) (*Response, error) {
	return c.Evaluate(ctx, Request{State: state, Questions: questions})
}

func (c *Client) Evaluate(ctx context.Context, request Request) (*Response, error) {
	if err := validateRequest(request); err != nil {
		return nil, err
	}
	if strings.TrimSpace(request.Model) == "" {
		request.Model = c.model
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("encode Jev request: %w", err)
	}

	for attempt := 1; attempt <= c.maxAttempts; attempt++ {
		response, retryAfter, err := c.do(ctx, payload)
		if err == nil {
			return response, nil
		}
		var apiErr *APIError
		if !errors.As(err, &apiErr) || !apiErr.Retryable() || attempt == c.maxAttempts {
			return nil, err
		}
		if err := sleep(ctx, retryDelay(attempt, retryAfter)); err != nil {
			return nil, err
		}
	}
	panic("unreachable")
}

func (c *Client) do(ctx context.Context, payload []byte) (*Response, time.Duration, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(payload))
	if err != nil {
		return nil, 0, fmt.Errorf("create Jev request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.apiKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "jev-go")

	httpResponse, err := c.httpClient.Do(request)
	if err != nil {
		return nil, 0, fmt.Errorf("call Jev: %w", err)
	}
	defer httpResponse.Body.Close()

	body, err := io.ReadAll(io.LimitReader(httpResponse.Body, maxResponseBytes+1))
	if err != nil {
		return nil, 0, fmt.Errorf("read Jev response: %w", err)
	}
	if len(body) > maxResponseBytes {
		return nil, 0, errors.New("Jev response exceeds 1 MiB")
	}
	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		apiErr := &APIError{StatusCode: httpResponse.StatusCode, Body: strings.TrimSpace(string(body))}
		return nil, parseRetryAfter(httpResponse.Header.Get("Retry-After")), apiErr
	}

	var response Response
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, 0, fmt.Errorf("decode Jev response: %w", err)
	}
	return &response, 0, nil
}

type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("Jev returned HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("Jev returned HTTP %d: %s", e.StatusCode, e.Body)
}

func (e *APIError) Retryable() bool {
	return e.StatusCode == http.StatusTooManyRequests || e.StatusCode == 529
}

func retryDelay(attempt int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		return retryAfter
	}
	return time.Duration(1<<(attempt-1)) * 250 * time.Millisecond
}

func parseRetryAfter(value string) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if date, err := http.ParseTime(value); err == nil {
		if delay := time.Until(date); delay > 0 {
			return delay
		}
	}
	return 0
}

func sleep(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
