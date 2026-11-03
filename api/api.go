package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/gcapell/replicant/model"
)

const BaseURL = "https://api.replicant.space/v1"

var apiKey string

func init() {
	apiKey = os.Getenv("API_KEY")
	if apiKey == "" {
		fmt.Fprintf(os.Stderr, "need $API_KEY")
		os.Exit(1)
	}
}

func get(url string, args ...interface{}) ([]byte, error) {
	url = fmt.Sprintf(url, args...)
	log.Printf("get %s", url)
	req, err := http.NewRequest("GET", BaseURL+url, nil)

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

func Get[T any](v *T, url string, args ...any) (*T, error) {
	data, err := get(url, args...)
	if err != nil {
		return nil, err
	}
	// log.Printf("response: %s", string(data))
	if err := json.Unmarshal(data, v); err != nil {
		return nil, err
	}
	return v, nil
}

func Stars(replicantID string) (*model.Stars, error) {
	return Get(new(model.Stars), "/replicants/%s/stars?per_page=30", replicantID)
}
func User() (*model.User, error) {
	return Get(new(model.User), "/accounts/me")
}

func Replicant(id string) (*model.Replicant, error) {
	return Get(new(model.Replicant), "/replicants/%s", id)
}

func Inventory(id string) (*model.Inventory, error) {
	return Get(new(model.Inventory), "/replicants/%s/inventory", id)
}

func Location(id string) (*model.Location, error) {
	return Get(new(model.Location), "/locations/%s", id)
}
