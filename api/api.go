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
		return nil, &HTTPError{Method: method, URL: url, Status: resp.Status, StatusCode: resp.StatusCode, Body: data}
	}
	return data, err
}

// HTTPError is a non-2xx response. Use errors.As to look at its body.
type HTTPError struct {
	Method, URL, Status string
	StatusCode          int
	Body                []byte
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%s %s: %s: %s", e.Method, e.URL, e.Status, e.Body)
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

// Stars returns the perPage stars nearest to replicantID, with positions.
func Stars(ctx context.Context, replicantID string, perPage int) (*model.Stars, error) {
	return Get(ctx, new(model.Stars), "/replicants/%s/stars?per_page=%d", replicantID, perPage)
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

// AttachDevice connects devices to carrier (e.g. drones onto a transport).
func AttachDevice(ctx context.Context, carrier string, devices ...string) error {
	return deviceCommand(ctx, carrier, map[string]any{"command": "attach", "targets": devices})
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

// SurveySystem tells an ami_survey_controller to have its drones scan every
// planet and moon in its system. The drones stay where they finish (no recall).
func SurveySystem(ctx context.Context, controller string) error {
	return deviceCommand(ctx, controller, map[string]any{
		"command":   "set_directive",
		"directive": "survey_system",
		"configuration": map[string]any{
			"planets": "all",
			"moons":   "all",
			"recall":  false,
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
		path := "/inventory?limit=50"
		if loc != "" {
			path += "&location=" + loc
		}
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

// ChangeOwner reassigns device to another replicant on the same account.
func ChangeOwner(ctx context.Context, device, replicant string) error {
	return deviceCommand(ctx, device, map[string]any{"command": "change_owner", "target": replicant})
}

// Stow places device in the hold of host (e.g. a vessel); both must be at the same location.
func Stow(ctx context.Context, device, host string) error {
	return deviceCommand(ctx, device, map[string]any{"command": "stow", "target": host})
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

// Deploy deploys a stowed device to its host's current location. It's sent to
// the stowed device itself.
func Deploy(ctx context.Context, device string) error {
	return deviceCommand(ctx, device, map[string]any{"command": "deploy"})
}

// Activate activates a deployed device, e.g. an ftl_relay.
func Activate(ctx context.Context, device string) error {
	return deviceCommand(ctx, device, map[string]any{"command": "activate"})
}

// SystemScan has a vessel scan the star system it's in.
func SystemScan(ctx context.Context, vessel string) (*model.SystemScan, error) {
	data, err := post(ctx, map[string]any{"command": "system_scan"}, "/devices/%s", vessel)
	if err != nil {
		return nil, err
	}
	v := new(model.SystemScan)
	if err := unmarshal(data, v); err != nil {
		return nil, fmt.Errorf("system_scan %s: %w: %s", vessel, err, data)
	}
	if v.EntryPoint == "" {
		return nil, fmt.Errorf("system_scan %s: no entry_point in response: %s", vessel, data)
	}
	return v, nil
}

// Compact collapses a modular device (e.g. a galactic_observatory) so it can be
// attached to a surge carrier. It takes hours.
func Compact(ctx context.Context, device string) error {
	return deviceCommand(ctx, device, map[string]any{"command": "compact"})
}

// Unfurl reassembles a compacted modular device. It takes hours.
func Unfurl(ctx context.Context, device string) error {
	return deviceCommand(ctx, device, map[string]any{"command": "unfurl"})
}

// Prospect has a galactic_observatory survey for new stars in direction (relative
// to its position; need not be normalised). A refusal because the area is too
// dense is an *HTTPError with status 400.
func Prospect(ctx context.Context, observatory string, direction model.Position) (*model.ProspectStarted, error) {
	data, err := post(ctx, map[string]any{
		"command":   "prospect",
		"direction": []float64{direction.X, direction.Y, direction.Z},
	}, "/devices/%s", observatory)
	if err != nil {
		return nil, err
	}
	v := new(model.ProspectStarted)
	if err := unmarshal(data, v); err != nil {
		return nil, fmt.Errorf("prospect %s: %w: %s", observatory, err, data)
	}
	return v, nil
}
