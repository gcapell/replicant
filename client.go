package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
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

func (c *Client) get(url string, args ...interface{}) ([]byte, error) {
	url = fmt.Sprintf(url, args...)
	log.Printf("get %s", url)
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

func (c *Client) getJSON(v any, url string, args ...any) error {
	data, err := c.get(url, args...)
	if err != nil {
		return err
	}
	log.Printf("response: %s", string(data))
	return json.Unmarshal(data, v)
}

func (c *Client) postJSON(v any, url string, args ...any) ([]byte, error) {
	url = fmt.Sprintf(url, args...)
	log.Printf("post %s", url)
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", c.APIEndpoint+url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Request failed with status code: %d (%s)", resp.StatusCode, string(body))
	}
	return io.ReadAll(resp.Body)
}

func (c *Client) user() (*User, error) {
	var m User
	if err := c.getJSON(&m, "/accounts/me"); err != nil {
		return nil, err
	}
	return &m, nil
}

func (c *Client) replicant(id string) (*Replicant, error) {
	var r Replicant
	if err := c.getJSON(&r, "/replicants/%s", id); err != nil {
		return nil, err
	}
	return &r, nil
}

func (c *Client) inventory(id string) (*Inventory, error) {
	var inventory Inventory
	if err := c.getJSON(&inventory, "/replicants/%s/inventory", id); err != nil {
		return nil, err
	}
	return &inventory, nil
}
func (c *Client) locationInfo(id string) (*Location, error) {
	var loc Location
	if err := c.getJSON(&loc, "/locations/%s", id); err != nil {
		return nil, err
	}
	return &loc, nil
}
func (c *Client) deployMiners(r *Replicant) error {
	for _, d := range r.StowedDevices {
		switch d.DeviceType {
		case miningDrone, surveyDrone:
		default:
			continue
		}
		if _, err := c.postJSON(deployCmd(), "/devices/%s", d.DeviceCode); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) search(id string) (*SearchResponse, error) {
	var response SearchResponse
	if body, err := c.postJSON(&Command{Command: "search"}, "/devices/%s", id); err != nil {
		return nil, err
	} else {
		if err := json.Unmarshal(body, &response); err != nil {
			return nil, err
		}
	}
	return &response, nil
}
