package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/model"
)

type decoder func([]byte) (any, error)

var registry = map[string]decoder{}

func register[T any](name string) {
	registry[name] = func(raw []byte) (any, error) {
		var v T
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, err
		}
		return &v, nil
	}
}

func decode(env api.Envelope) (any, error) {
	dec, ok := registry[env.Event]
	if !ok {
		return nil, fmt.Errorf("unknown event type %q", env.Event)
	}
	return dec(env.Payload)
}

func main() {
	err := api.SSEStream(context.Background(), func(env api.Envelope) error {
		//fmt.Println(env.Event, env.ID, env.DeviceCode, env.DeviceType)
		o, err := decode(env)
		if err != nil {
			fmt.Printf("could not decode %s %v: payload %s\n", env.Event, err, string(env.Payload))
		} else {
			fmt.Printf("Decoded %s, %s: %+v\n", env.Event, env.DeviceCode, o)
		}
		return nil
	})
	if err != nil {
		log.Fatalf("read events: %v", err)
	}
}

func init() {
	register[model.MiningDigest]("ami.mining.digest")
}
