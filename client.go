package sls

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

// Client is used to interact with the logging server.
type Client struct {
	client        HTTPClient
	apiKey        string
	url           string
	buf           []string
	mu            sync.Mutex
	errCh         chan error
	flushInterval time.Duration
}

// HTTPClient is satisfied by *http.Client but enables us to pass in
// alternative http clients as well with different features (such as automatic
// retries or allowing host-based communication over a LAN).
type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

// NewClient for interacting with sls.
func NewClient(url, apiKey string) *Client {
	// These settings are lifted from Hashicorp's cleanhttp package.
	httpClient := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
				DualStack: true,
			}).DialContext,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			MaxIdleConnsPerHost:   -1,
			DisableKeepAlives:     true,
		},
		Timeout: 10 * time.Second,
	}
	httpClient.Timeout = 10 * time.Second
	c := &Client{
		client: httpClient,
		url:    url,
		apiKey: apiKey,
	}
	return c
}

func (c *Client) WithHTTPClient(client HTTPClient) *Client {
	c.client = client
	return c
}

// WithFlushInterval specifies how long to wait before flushing the buffer to
// the log server. This returns a function which flushes the client and should
// be called with defer before main exits.
func (c *Client) WithFlushInterval(dur time.Duration) (*Client, func()) {
	c.flushInterval = dur
	go func() {
		for range time.Tick(dur) {
			c.flush()
		}
	}()
	return c, c.flush
}

// marshalBuffer to JSON. If the buffer is empty, marshalBuffer reports nil.
// This is not thread-safe, so protect any call with a mutex.
func (c *Client) marshalBuffer() ([]byte, error) {
	if len(c.buf) == 0 {
		return nil, nil
	}
	byt, err := json.Marshal(c.buf)
	c.buf = []string{}
	if err != nil {
		return nil, fmt.Errorf("marshal log buffer: %w", err)
	}
	return byt, nil
}

// flush the log buffer to the server. This happens automatically over time if
// WithFlushInterval is called.
func (c *Client) flush() {
	c.mu.Lock()
	defer c.mu.Unlock()

	byt, err := c.marshalBuffer()
	if err != nil {
		c.sendErr(fmt.Errorf("marshal buffer: %w", err))
		return
	}
	if len(byt) == 0 {
		return
	}
	req, err := http.NewRequest("POST", c.url+"/log", bytes.NewReader(byt))
	if err != nil {
		c.sendErr(fmt.Errorf("new request: %w", err))
		return
	}
	req.Header.Set("X-API-Key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		c.sendErr(fmt.Errorf("do: %w", err))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		c.sendErr(fmt.Errorf("expected 200, got %d", resp.StatusCode))
		return
	}
}

// Err is a convenience function that wraps an error channel.
func (c *Client) Err() <-chan error {
	if c.errCh == nil {
		c.errCh = make(chan error)
	}
	return c.errCh
}

func (c *Client) sendErr(err error) {
	if c.errCh == nil {
		return
	}
	c.errCh <- err
}

// Log to sls. This is threadsafe.
func (c *Client) Log(s string) {
	c.mu.Lock()
	if c.flushInterval > 0 {
		c.buf = append(c.buf, s)
		c.mu.Unlock()
		return
	}

	// Don't buffer, flush immediately. Flush locks the mutex, so we unlock
	// before flush is called.
	c.buf = []string{s}
	c.mu.Unlock()
	c.flush()
}

// Write satisfies the io.Writer interface, so a client can be a drop-in
// replacement for stdout. Logs are written to the external service on a
// best-effort basis.
func (c *Client) Write(byt []byte) (int, error) {
	c.Log(string(byt))
	return len(byt), nil
}
