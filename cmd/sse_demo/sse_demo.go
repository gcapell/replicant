package main

import (
	"context"
	"fmt"
	"log"

	"github.com/gcapell/replicant/api"
	"github.com/sanity-io/litter"
)

var boringEvents = map[string]bool{
	"ami.mining.digest": true,
	"ami.survey.digest": true,
	"site.depleted":     true,
}

func main() {
	needsLineBreak := false
	err := api.SSEStream(context.Background(), "1793829791261-0", func(env api.Envelope, o any) error {
		if boringEvents[env.Event] {
			fmt.Printf("%s, ", env.Event)
			needsLineBreak = true
			return nil
		}
		if needsLineBreak {
			fmt.Println()
			needsLineBreak = false
		}
		fmt.Printf("%s %s %s %s %s\n", env.Event, env.ID, env.DeviceCode, env.DeviceType, litter.Sdump(o))
		return nil
	})
	if err != nil {
		log.Fatalf("read events: %v", err)
	}
}
