package main

import (
	"cmp"
	"context"
	"fmt"
	"iter"
	"maps"
	"os"
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

// Usage: summarize_devices [device_type]
func main() {
	ctx := context.Background()
	path := "/devices"
	switch len(os.Args) {
	case 1:
	case 2:
		path += "?device_type=" + os.Args[1]
	default:
		fmt.Fprintln(os.Stderr, "usage: summarize_devices [device_type]")
		os.Exit(2)
	}
	byType := make(map[string][]model.Device)
	for list, err := range api.Paged[model.DeviceList](ctx, path) {
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
			// Count devices per status, e.g. `"OTHYONAL" idle monitoring(3)`.
			counts := map[string]int{}
			for _, d := range devices {
				counts[d.Status]++
			}
			var parts []string
			for status, n := range sortedMap(counts) {
				if n > 1 {
					status = fmt.Sprintf("%s(%d)", status, n)
				}
				parts = append(parts, status)
			}
			fmt.Printf("  %q %s\n", star, strings.Join(parts, " "))
		}
		fmt.Println()
	}
}
