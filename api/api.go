package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gcapell/replicant/model"
	jsoniter "github.com/json-iterator/go"
	"github.com/json-iterator/go/extra"
)

const BaseURL = "https://api.replicant.space/v1"

var unmarshal = jsoniter.ConfigCompatibleWithStandardLibrary.Unmarshal
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

func get(ctx context.Context, url string, args ...interface{}) ([]byte, error) {
	url = fmt.Sprintf(url, args...)
	log.Printf("get %s", url)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, BaseURL+url, nil)

	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Request failed with status code: %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func Paged[T model.Pageable](ctx context.Context, url string) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		cursor := 0
		for {
			var o T
			data, err := get(ctx, url+"?cursor=%d", cursor)
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

func Device(ctx context.Context, id string) (*model.Device, error) {
	return Get(ctx, new(model.Device), "/devices/%s", id)
}

func deviceCommand(ctx context.Context, id string, args map[string]any) error {
	encodedArgs, err := json.Marshal(args)
	if err != nil {
		return err
	}
	log.Printf("encoded: %q", encodedArgs)
	client := &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("/devices/%s", id)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, BaseURL+url, bytes.NewReader(encodedArgs))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		return nil
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("err %s reading POST response", err)
	}
	return fmt.Errorf("unexpected status: %s, %s", resp.Status, string(body))
}

func DetachDevice(ctx context.Context, carrier string, devices ...string) error {
	return deviceCommand(ctx, carrier, map[string]any{"command": "detach", "targets": devices})
}
