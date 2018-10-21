package sls

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/pkg/errors"
)

// Client is used to interact with the logging server.
type Client struct {
	client *http.Client
	apiKey string
	url    string
	errCh  chan<- error
}

// NewClient for interacting with sls.
func NewClient(url, apiKey string) *Client {
	return &Client{
		client: &http.Client{Timeout: 5 * time.Second},
		url:    url,
		apiKey: apiKey,
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
	byt, err := json.Marshal(buf)
	if err != nil {
		c.sendErr(errors.Wrap(err, "marshal log buffer"))
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
	return
}

// Write satisfies the io.Writer interface, so a client can be a drop-in
// replacement for stdout. Logs are written to the external service on a
// best-effort basis.
func (c *Client) Write(byt []byte) (int, error) {
	go c.Log([]string{string(byt)})
	return len(byt), nil
}
