// Package auto holds higher-level helpers shared by the automation commands,
// built on the thin api package.
package auto

import (
	"context"
	"regexp"
	"strings"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/model"
)

// DevicesInSystem returns all devices of deviceType anywhere in the star system.
func DevicesInSystem(ctx context.Context, star, deviceType string) ([]model.Device, error) {
	return listDevices(ctx, "/devices?location="+star+"&device_type="+deviceType+"&limit=50")
}

// DevicesAt returns all devices of deviceType at exactly loc (e.g. "SOL-4-L4").
func DevicesAt(ctx context.Context, loc, deviceType string) ([]model.Device, error) {
	return listDevices(ctx, "/devices?location="+loc+"&device_type="+deviceType+"&limit=50")
}

// Devices returns all our devices of deviceType.
func Devices(ctx context.Context, deviceType string) ([]model.Device, error) {
	return listDevices(ctx, "/devices?device_type="+deviceType+"&limit=50")
}

// Everything returns all our devices, of every type, at loc (a star or an exact location).
func Everything(ctx context.Context, loc string) ([]model.Device, error) {
	return listDevices(ctx, "/devices?location="+loc+"&limit=50")
}

// Note: path goes through Sprintf in api, so it must not contain '%'.
func listDevices(ctx context.Context, path string) ([]model.Device, error) {
	var devices []model.Device
	for list, err := range api.Paged[model.DeviceList](ctx, path) {
		if err != nil {
			return nil, err
		}
		devices = append(devices, list.Devices...)
	}
	return devices, nil
}

// StarOf returns the star part of a location code ("MIRETH" for "MIRETH-5-2").
func StarOf(loc string) string {
	star, _, _ := strings.Cut(loc, "-")
	return star
}

// planetOrMoon matches the part of a location after "STAR-" for a planet ("2")
// or moon ("2-3"), but not belts ("BELT-1"), Lagrange points ("5-L4"), or sites.
var planetOrMoon = regexp.MustCompile(`^\d+(-\d+)?$`)

// IsPlanetOrMoon reports whether loc is a planet or moon of star.
func IsPlanetOrMoon(star, loc string) bool {
	rest, ok := strings.CutPrefix(loc, star+"-")
	return ok && planetOrMoon.MatchString(rest)
}
