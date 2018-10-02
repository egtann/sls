package sls

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/eapache/go-resiliency/retrier"
	"github.com/pkg/errors"
)

// Client is used to interact with the logging server.
type Client struct {
	log    Logger
	client *http.Client
	apiKey string
	url    string
	len    int
	maxLen int
	buf    []string
}

// NewClient for interacting with sls.
func NewClient(
	log Logger,
	url, apiKey string,
	timeout time.Duration,
	maxLen int,
) *Client {
	return &Client{
		log:    log,
		client: &http.Client{Timeout: timeout},
		url:    url,
		apiKey: apiKey,
		maxLen: maxLen,
	}
}

// Log to sls and clear the buffer. This is threadsafe.
func (c *Client) Log(buf []string) error {
	byt, err := json.Marshal(buf)
	if err != nil {
		return errors.Wrap(err, "marshal log buffer")
	}
	req, err := http.NewRequest("POST", c.url+"/log", bytes.NewReader(byt))
	if err != nil {
		return errors.Wrap(err, "new request")
	}
	req.Header.Set("X-API-Key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return errors.Wrap(err, "do")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("expected 200, got %d", resp.StatusCode)
	}
	return nil
}

// Write satisfies the io.Writer interface, so a client can be a drop-in
// replacement for stdout. Logs are written to the external service on a
// best-effort basis. Write will attempt to contact sls 3 times over a period
// of several seconds on network errors. After 3 failures, the client drops the
// logs from sls, but both the error and the dropped logs are recorded
// internally.
func (c *Client) Write(byt []byte) (int, error) {
	c.buf = append(c.buf, string(byt))
	c.len += len(byt)
	if c.len < c.maxLen {
		return len(byt), nil
	}
	go func(buf []string) {
		r := retrier.New(retrier.ExponentialBackoff(3, time.Second), nil)
		err := r.Run(func() error {
			return c.Log(buf)
		})
		if err == nil {
			return
		}
		c.log.Printf("failed log: %s\n", err)
		for _, l := range buf {
			c.log.Printf("\t> %s\n", l)
		}
	}(c.buf)
	c.buf = nil
	return len(byt), nil
}

// Flush buffer to logs.
func (c *Client) Flush() {
	if len(c.buf) == 0 {
		return
	}
	if err := c.Log(c.buf); err != nil {
		c.log.Printf("failed flush: %s\n", err)
		for _, l := range c.buf {
			c.log.Printf("\t> %s\n", l)
		}
	}
}
