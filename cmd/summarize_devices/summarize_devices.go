package main

import (
	"cmp"
	"context"
	"fmt"
	"iter"
	"maps"
	"slices"
	"strings"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/model"
)

func sortedMap[K cmp.Ordered, V any](m map[K]V) iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for _, k := range slices.Sorted(maps.Keys(m)) {
			if !yield(k, m[k]) {
				return
			}
		}
	}
}

func main() {
	ctx := context.Background()
	byType := make(map[string][]model.Device)
	for list, err := range api.Paged[model.DeviceList](ctx, "/devices") {
		if err != nil {
			fmt.Println("Error:", err)
			return
		}
		for _, d := range list.Devices {
			byType[d.DeviceType] = append(byType[d.DeviceType], d)
		}
	}
	for deviceType, devices := range sortedMap(byType) {
		fmt.Println(deviceType)
		byStar := make(map[string][]model.Device)
		for _, d := range devices {
			star, _, _ := strings.Cut(d.Location, "-")
			byStar[star] = append(byStar[star], d)
		}
		for star, devices := range sortedMap(byStar) {
			fmt.Printf("  %q\n", star)
			for _, d := range devices {
				fmt.Println("    ", d.DeviceCode, d.Status)
			}
		}
		fmt.Println()
	}
}
