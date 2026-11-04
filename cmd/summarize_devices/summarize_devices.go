package main

import (
	"fmt"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/model"
)

func main() {
	for list, err := range api.Paged[model.DeviceList]("/devices") {
		if err != nil {
			fmt.Println("Error:", err)
			return
		}
		fmt.Println("list", list)
	}
}
