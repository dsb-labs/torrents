// Package client provides an HTTP client and canonical domain types for the torrents API.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/dsb-labs/torrents/internal/server/api"
)

// The Client type talks to a torrents server over HTTP.
type Client struct {
	base *url.URL
	http *http.Client
}

// New returns a Client that targets the server at the given base address.
// An invalid URL is rejected so callers see the problem at construction time.
func New(address string) (*Client, error) {
	u, err := url.Parse(address)
	if err != nil {
		return nil, fmt.Errorf("failed to parse address: %w", err)
	}

	return &Client{
		base: u,
		http: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func do[Response any](ctx context.Context, c *Client, method, endpoint string, request any) (Response, error) {
	var zero Response

	var body io.Reader
	if request != nil {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(request); err != nil {
			return zero, fmt.Errorf("failed to encode request: %w", err)
		}
		body = &buf
	}

	target := *c.base
	target.Path = path.Join(target.Path, endpoint)

	req, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return zero, fmt.Errorf("failed to construct request: %w", err)
	}

	if request != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return zero, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusOK && resp.StatusCode <= http.StatusIMUsed {
		var response Response
		if err = json.NewDecoder(resp.Body).Decode(&response); err != nil {
			return zero, fmt.Errorf("failed to decode response: %w", err)
		}

		return response, nil
	}

	var apiErr api.ErrorResponse
	_ = json.NewDecoder(resp.Body).Decode(&apiErr)
	apiErr.Status = resp.StatusCode

	return zero, apiErr
}

// IsNotFound reports whether err is a server-side ErrorResponse with a 404 status.
func IsNotFound(err error) bool {
	var apiErr api.ErrorResponse
	if !errors.As(err, &apiErr) {
		return false
	}

	return apiErr.Status == http.StatusNotFound
}

// IsConflict reports whether err is a server-side ErrorResponse with a 409 status.
func IsConflict(err error) bool {
	var apiErr api.ErrorResponse
	if !errors.As(err, &apiErr) {
		return false
	}

	return apiErr.Status == http.StatusConflict
}

// IsBadRequest reports whether err is a server-side ErrorResponse with a 400 status.
func IsBadRequest(err error) bool {
	var apiErr api.ErrorResponse
	if !errors.As(err, &apiErr) {
		return false
	}

	return apiErr.Status == http.StatusBadRequest
}
