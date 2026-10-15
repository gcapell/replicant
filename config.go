package main

import (
	"fmt"
	"io"
	"net/http"
)

const (
	Token = "J9RbhJ2aoZhOInX5j6lBUUH1l8oO4aKHPcSbA94OeacU9BRuRBiV-2dk1TsXa9je"

	Code        = "4698E8E6"
	APIEndpoint = "https://api.replicant.space/v1"
)

func main() {
	fmt.Printf("token:%s, Code:%s, APIEndpoint:%s\n", Token, Code, APIEndpoint)
	client := &http.Client{}
	req, err := http.NewRequest("GET", APIEndpoint+"/accounts/me", nil)

	if err != nil {
		panic(err)
	}
	req.Header.Set("Authorization", "Bearer "+Token)
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		panic(fmt.Sprintf("Request failed with status code: %d", resp.StatusCode))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(body))
}
