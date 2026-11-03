package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/model"
)

func main() {
	err := api.SSEStream(context.Background(), func(env api.Envelope) error {
		fmt.Println(env.Event, env.ID, env.DeviceCode, env.DeviceType)

		switch env.Event {
		case "ami.mining.digest":
			var d model.MiningDigest
			if err := json.Unmarshal(env.Payload, &d); err != nil {
				return err
			}
			fmt.Printf("MiningDigest: %+v\n", d)
		}
		return nil
	})
	if err != nil {
		log.Fatalf("read events: %v", err)
	}
}
