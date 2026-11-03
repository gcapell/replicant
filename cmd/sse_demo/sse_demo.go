package main

import (
	"context"
	"fmt"
	"log"

	"github.com/gcapell/replicant/api"
)

func main() {
	err := api.SSEStream(context.Background(), func(env api.Envelope, o any) error {
		fmt.Println(env.Event, env.ID, env.DeviceCode, env.DeviceType, o)
		return nil
	})
	if err != nil {
		log.Fatalf("read events: %v", err)
	}
	if err != nil {
		log.Fatalf("read events: %v", err)
	}
}
