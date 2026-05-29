package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	server string
	token  string
	http   *http.Client
}

func newClient(cfg *Config) *Client {
	return &Client{
		server: cfg.Server,
		token:  cfg.Token,
		http:   &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Client) do(method, path string, body any) ([]byte, int, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, c.server+path, bodyReader)
	if err != nil {
		return nil, 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	return data, resp.StatusCode, err
}

func (c *Client) get(path string) ([]byte, error) {
	data, status, err := c.do("GET", path, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, apiError(data, status)
	}
	return data, nil
}

func (c *Client) post(path string, body any) ([]byte, error) {
	data, status, err := c.do("POST", path, body)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, apiError(data, status)
	}
	return data, nil
}

func (c *Client) patch(path string, body any) ([]byte, error) {
	data, status, err := c.do("PATCH", path, body)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, apiError(data, status)
	}
	return data, nil
}

func apiError(body []byte, status int) error {
	var e struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &e); err == nil && e.Error != "" {
		return fmt.Errorf("%s (HTTP %d)", e.Error, status)
	}
	return fmt.Errorf("HTTP %d", status)
}
