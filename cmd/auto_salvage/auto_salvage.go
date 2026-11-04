package main

import (
	"context"
	"fmt"
	"log"

	"github.com/gcapell/replicant/api"
	"github.com/sanity-io/litter"
)

var transportID = "E62C8DC1" // drones are attached to this.

func main() {
	ctx := context.Background()
	t, err := api.Device(ctx, transportID)
	if err != nil {
		log.Fatal(err)
	}
	if false {
		fmt.Printf("Transport %s\n", litter.Sdump(t))
	}

	// Detach any attached mining drones.
	var targets []string
	for _, d := range t.AttachedDevices {
		switch d.DeviceType {
		case "mining_drone", "ami_mining_controller":
			targets = append(targets, d.DeviceCode)
		}
	}
	log.Printf("targets: %s", targets)
	if len(targets) > 0 {
		if err := api.DetachDevice(ctx, transportID, targets...); err != nil {
			log.Fatal(err)
		}
	}

}
