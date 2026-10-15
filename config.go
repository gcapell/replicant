package main

import (
	"fmt"
)

func main() {
	client := NewClient(Token, APIEndpoint)

	m, err := client.user()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	fmt.Printf("%+v\n", m)
}
