package sls

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/pkg/errors"
)

// Client is used to interact with the logging server.
type Client struct {
	client *http.Client
	apiKey string
	url    string
	buf    []string
	mu     sync.RWMutex
	errCh  chan<- error
}

// NewClient for interacting with sls.
func NewClient(url, apiKey string, flushInterval time.Duration) *Client {
	c := &Client{
		client: &http.Client{Timeout: 2 * time.Second},
		url:    url,
		apiKey: apiKey,
	}
	go func() {
		for range time.Tick(flushInterval) {
			c.flush()
		}
	}()
	return c
}

func (c *Client) marshalBuffer() ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	byt, err := json.Marshal(c.buf)
	c.buf = []string{}
	if err != nil {
		return nil, errors.Wrap(err, "marshal log buffer")
	}
	return byt, nil
}

func (c *Client) flush() {
	byt, err := c.marshalBuffer()
	if err != nil {
		c.sendErr(errors.Wrap(err, "marshal buffer"))
		return
	}
	if len(byt) == 0 {
		return
	}
	req, err := http.NewRequest("POST", c.url+"/log", bytes.NewReader(byt))
	if err != nil {
		c.sendErr(errors.Wrap(err, "new request"))
		return
	}
	req.Header.Set("X-API-Key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		c.sendErr(errors.Wrap(err, "do"))
		return
	}
	if resp.StatusCode != http.StatusOK {
		c.sendErr(fmt.Errorf("expected 200, got %d", resp.StatusCode))
		return
	}
}

func (c *Client) WithErrorChannel(ch chan<- error) *Client {
	c.errCh = ch
	return c
}

func (c *Client) sendErr(err error) {
	if c.errCh == nil {
		return
	}
	c.errCh <- err
}

// Log to sls. This is threadsafe.
func (c *Client) Log(buf []string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	c.buf = append(c.buf, buf...)
}

// Write satisfies the io.Writer interface, so a client can be a drop-in
// replacement for stdout. Logs are written to the external service on a
// best-effort basis.
func (c *Client) Write(byt []byte) (int, error) {
	c.Log([]string{string(byt)})
	return len(byt), nil
}
