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
	fmt.Printf("client: %+v\n", m)

	r, err := client.replicant(m.Replicants[0].Code)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	fmt.Printf("replicant: %+v\n", r)
}
