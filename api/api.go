package api

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"iter"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gcapell/replicant/model"
	jsoniter "github.com/json-iterator/go"
	"github.com/json-iterator/go/extra"
)

const BaseURL = "https://api.replicant.space/v1"

var (
	unmarshal = jsoniter.ConfigCompatibleWithStandardLibrary.Unmarshal
	marshal   = jsoniter.ConfigCompatibleWithStandardLibrary.Marshal
	client    = &http.Client{Timeout: 10 * time.Second}
)
var apiKey string

func init() {
	extra.SetNamingStrategy(extra.LowerCaseWithUnderscores)
}

func init() {
	apiKey = os.Getenv("API_KEY")
	if apiKey == "" {
		fmt.Fprintf(os.Stderr, "need $API_KEY")
		os.Exit(1)
	}
}

func get(ctx context.Context, url string, args ...any) ([]byte, error) {
	return do(ctx, http.MethodGet, nil, url, args...)
}

// post JSON-encodes body and POSTs it to url (formatted with args).
func post(ctx context.Context, body any, url string, args ...any) ([]byte, error) {
	return do(ctx, http.MethodPost, body, url, args...)
}

func do(ctx context.Context, method string, body any, url string, args ...any) ([]byte, error) {
	url = fmt.Sprintf(url, args...)
	var r io.Reader
	if body != nil {
		encoded, err := marshal(body)
		if err != nil {
			return nil, err
		}
		log.Printf("%s %s %s", method, url, encoded)
		r = bytes.NewReader(encoded)
	} else {
		log.Printf("%s %s", method, url)
	}
	req, err := http.NewRequestWithContext(ctx, method, BaseURL+url, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("%s %s: %s: %s", method, url, resp.Status, data)
	}
	return data, err
}

func Paged[T model.Pageable](ctx context.Context, url string) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		cursor := 0
		sep := "?"
		if strings.Contains(url, "?") {
			sep = "&"
		}
		for {
			var o T
			data, err := get(ctx, url+sep+"cursor=%d", cursor)
			if err != nil {
				yield(o, err)
				return
			}

			if err := unmarshal(data, &o); err != nil {
				yield(o, err)
				return
			}
			if !yield(o, nil) {
				return
			}
			var ok bool
			cursor, ok = o.Cursor()
			if !ok {
				return
			}
		}
	}
}

func Get[T any](ctx context.Context, v *T, url string, args ...any) (*T, error) {
	data, err := get(ctx, url, args...)
	if err != nil {
		return nil, err
	}
	// log.Printf("response: %s", string(data))
	if err := unmarshal(data, v); err != nil {
		return nil, err
	}
	return v, nil
}

func Stars(ctx context.Context, replicantID string) (*model.Stars, error) {
	return Get(ctx, new(model.Stars), "/replicants/%s/stars?per_page=30", replicantID)
}
func User(ctx context.Context) (*model.User, error) {
	return Get(ctx, new(model.User), "/accounts/me")
}

func Replicant(ctx context.Context, id string) (*model.Replicant, error) {
	return Get(ctx, new(model.Replicant), "/replicants/%s", id)
}

func Inventory(ctx context.Context, id string) (*model.Inventory, error) {
	return Get(ctx, new(model.Inventory), "/replicants/%s/inventory", id)
}

func Location(ctx context.Context, id string) (*model.Location, error) {
	return Get(ctx, new(model.Location), "/locations/%s", id)
}

// Locations fetches a summary of every known location.
func Locations(ctx context.Context) (*model.LocationList, error) {
	return Get(ctx, new(model.LocationList), "/locations")
}

func Device(ctx context.Context, id string) (*model.Device, error) {
	return Get(ctx, new(model.Device), "/devices/%s", id)
}

func deviceCommand(ctx context.Context, id string, args map[string]any) error {
	_, err := post(ctx, args, "/devices/%s", id)
	return err
}

func DetachDevice(ctx context.Context, carrier string, devices ...string) error {
	return deviceCommand(ctx, carrier, map[string]any{"command": "detach", "targets": devices})
}

// AdoptDevice tells an AMI controller to adopt the given devices.
func AdoptDevice(ctx context.Context, controller string, devices ...string) error {
	return deviceCommand(ctx, controller, map[string]any{"command": "adopt", "targets": devices})
}

func Salvage(ctx context.Context, controller string, site string) error {
	return deviceCommand(ctx, controller, map[string]any{
		"command":   "set_directive",
		"directive": "gather_salvage",
		"configuration": map[string]any{
			"location": site,
			"recall":   false,
		},
	})
}

// InventoryAt returns your stockpiles at every location under loc, which may be
// a star code (whole system) or a specific location.
func InventoryAt(ctx context.Context, loc string) ([]model.LocationInventory, error) {
	var all []model.LocationInventory
	cursor := ""
	for {
		// Note: path goes through Sprintf in do, so it must not contain '%'.
		path := "/inventory?limit=50&location=" + loc
		if cursor != "" {
			path += "&cursor=" + cursor
		}
		var page model.InventoryList
		if _, err := Get(ctx, &page, path); err != nil {
			return nil, err
		}
		all = append(all, page.Locations...)
		if page.NextCursor == nil || *page.NextCursor == "" {
			return all, nil
		}
		cursor = *page.NextCursor
	}
}

// Travel sends a device to destination (a location code).
func Travel(ctx context.Context, device, destination string) error {
	return deviceCommand(ctx, device, map[string]any{"command": "travel", "destination": destination})
}

// CollectResources loads resources from the device's current location into its cargo.
func CollectResources(ctx context.Context, device string, resources map[model.ResourceType]int) error {
	return deviceCommand(ctx, device, map[string]any{"command": "collect_resources", "resources": resources})
}

// DepositAllResources unloads all of the device's cargo at its current location.
func DepositAllResources(ctx context.Context, device string) error {
	return deviceCommand(ctx, device, map[string]any{"command": "deposit_resources"})
}
