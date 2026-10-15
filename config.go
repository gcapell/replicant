package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	Token = "J9RbhJ2aoZhOInX5j6lBUUH1l8oO4aKHPcSbA94OeacU9BRuRBiV-2dk1TsXa9je"

	Code        = "4698E8E6"
	APIEndpoint = "https://api.replicant.space/v1"
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

func (c *Client) get(url string) (string, error) {

	req, err := http.NewRequest("GET", c.APIEndpoint+url, nil)

	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Request failed with status code: %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

type Message struct {
	ID             string      `json:"id"`
	CreatedAt      string      `json:"created_at"`
	Email          string      `json:"email"`
	XPTotal        int         `json:"experience_points_total"`
	Replicants     []Replicant `json:"replicants"`
	Status         string      `json:"status"`
	UnreadMessages int         `json:"unread_message_count"`
}

type Replicant struct {
	CurrentLocation string `json:"current_location"`
	CurrentStar     string `json:"current_star"`
	CreatedAt       string `json:"created_at"`
	Code            string `json:"replicant_code"`
}

func main() {
	client := NewClient(Token, APIEndpoint)

	me, err := client.get("/accounts/me")
	if err != nil {
		panic(err)
	}
	fmt.Println(string(me))
	var m Message

	err = json.Unmarshal([]byte(me), &m)
	if err != nil {
		panic(err)
	}
	fmt.Println(m)
}
