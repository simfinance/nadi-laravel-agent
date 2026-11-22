package enrollment

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	EnrollPath      = "/api/v1/nadi/agent/enroll"
	maxResponseBody = 4 << 20
)

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

func NewClient(baseURL string, timeout time.Duration) *Client {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          20,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
	}
}

func (c *Client) Enroll(ctx context.Context, body []byte) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+EnrollPath, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create Enrollment request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")

	response, err := c.HTTP.Do(request)
	if err != nil {
		return nil, fmt.Errorf("send Enrollment request: %w", err)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBody+1))
	if err != nil {
		return nil, fmt.Errorf("read Enrollment response: %w", err)
	}
	if len(responseBody) > maxResponseBody {
		return nil, fmt.Errorf("Enrollment response exceeds %d bytes", maxResponseBody)
	}

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		message := strings.TrimSpace(string(responseBody))
		if len(message) > 512 {
			message = message[:512]
		}
		return nil, fmt.Errorf("Central Enrollment returned HTTP %d: %s", response.StatusCode, message)
	}

	return responseBody, nil
}

func (c *Client) CloseIdleConnections() {
	c.HTTP.CloseIdleConnections()
}
