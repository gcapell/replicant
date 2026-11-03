package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"go.jetify.com/sse"
)

func main() {
	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		log.Fatal("need $API_KEY")
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"https://api.replicant.space/v1/events/stream", nil)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer resp.Body.Close()

	// The checks the library can't do for you:
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		log.Fatalf("unexpected status %s: %s", resp.Status, body)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		log.Fatalf("unexpected content-type %q", ct)
	}

	dec := sse.NewDecoder(resp.Body)
	for {
		var ev sse.Event
		if err := dec.Decode(&ev); err != nil {
			if errors.Is(err, io.EOF) {
				log.Printf("stream closed (last id %q)", dec.LastEventID())
				return
			}
			log.Fatalf("decode: %v", err)
		}
		fmt.Printf("id=%s event=%s data=%v\n", ev.ID, ev.Event, ev.Data)
	}
}
