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
}

// NewClient for interacting with sls.
func NewClient(url, apiKey string) *Client {
	return &Client{
		client: &http.Client{Timeout: 5 * time.Second},
		url:    url,
		apiKey: apiKey,
	}
}

// Log to sls. This is threadsafe.
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
	err := c.Log([]string{string(byt)})
	return len(byt), err
}
