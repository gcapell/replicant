package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Client struct {
	Token       string
	APIEndpoint string
	client      *http.Client
}

func NewClient(token, apiEndpoint string) *Client {
	return &Client{
		Token:       token,
		APIEndpoint: apiEndpoint,
		client:      &http.Client{},
	}
}

func (c *Client) get(url string) ([]byte, error) {

	req, err := http.NewRequest("GET", c.APIEndpoint+url, nil)

	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Request failed with status code: %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func (c *Client) user() (*User, error) {
	data, err := c.get("/accounts/me")
	if err != nil {
		return nil, err
	}
	var m User

	err = json.Unmarshal(data, &m)
	if err != nil {
		return nil, err
	}
	return &m, nil
}
