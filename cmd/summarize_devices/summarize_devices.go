package main

import (
	"fmt"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/model"
)

func main() {
	lists, err := api.Paged[model.DeviceList]("/devices")
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	for _, page := range lists {
		fmt.Println("page", page)
	}
}
