package control

import (
	"net"
	"net/http"
	"time"
)

type Client struct {
	BaseURL string
	Token   string
	PollURL string
	HTTP    *http.Client
}

func NewClient(baseURL string, token string, timeout time.Duration) *Client {
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
		BaseURL: baseURL,
		Token:   token,
		HTTP: &http.Client{
			Timeout:   timeout,
			Transport: transport,
			CheckRedirect: func(request *http.Request, previous []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (c *Client) SetToken(token string) {
	c.Token = token
}

func (c *Client) SetPollURL(pollURL string) {
	c.PollURL = pollURL
}
