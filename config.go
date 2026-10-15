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
	rep0 := m.Replicants[0].Code

	inventory, err := client.inventory(rep0)
	if err != nil {
		fmt.Println("inventory:", err)
		return
	}

	fmt.Printf("inventory: %+v\n", inventory)

	r, err := client.replicant(rep0)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	fmt.Printf("replicant: %+v\n", r)
	if err := client.deployMiners(r); err != nil {
		fmt.Println("deploying miners:", err)
		return
	}
	fmt.Printf("deployed miners\n")

}
