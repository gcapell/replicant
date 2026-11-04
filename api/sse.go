package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gcapell/replicant/model"
	jsoniter "github.com/json-iterator/go"
)

func decode(env Envelope) (any, error) {
	dec, ok := model.EventRegistry[env.Event]
	if !ok {
		return nil, fmt.Errorf("unknown event type %q", env.Event)
	}
	return dec(env.Payload)
}

type rawEvent struct {
	ID    string
	Event string
	Data  []byte // data lines joined with "\n", untouched
}

type Envelope struct {
	Version       int
	ID            string
	Category      string
	Event         string
	CreatedAt     time.Time
	DeviceCode    string
	DeviceType    string
	ReplicantCode string
	Star          string
	Location      string
	Payload       json.RawMessage
}

// readEvents calls fn for each complete event until the stream ends or fn errors.
func readEvents(r io.Reader, fn func(Envelope, any) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20) // allow lines up to 1 MiB

	var ev rawEvent
	var data bytes.Buffer
	lines := 0
	lastID := ""
	unmarshal := jsoniter.ConfigCompatibleWithStandardLibrary.Unmarshal

	for sc.Scan() {
		line := sc.Text() // Scanner strips the trailing \n or \r\n

		if line == "" { // blank line: dispatch
			if lines > 0 {
				ev.ID = lastID
				ev.Data = append([]byte(nil), data.Bytes()...) // copy; buffer is reused
				var env Envelope
				if err := unmarshal(ev.Data, &env); err != nil {
					return fmt.Errorf("envelope: %w", err)
				}
				o, err := decode(env)
				if err != nil {
					return fmt.Errorf("decode %s %s %s: %w", env.Event, ev.ID, string(env.Payload), err)
				}
				if err := fn(env, o); err != nil {
					return err
				}
			}
			ev, lines = rawEvent{}, 0
			data.Reset()
			continue
		}
		if line[0] == ':' { // comment / keepalive
			continue
		}

		field, val, _ := strings.Cut(line, ":")
		val = strings.TrimPrefix(val, " ") // strip at most one leading space

		switch field {
		case "data":
			if lines > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(val)
			lines++
		case "event":
			ev.Event = val
		case "id":
			if !strings.ContainsRune(val, 0) {
				lastID = val
			}
		}
	}
	return sc.Err() // nil on a clean EOF
}

func SSEStream(ctx context.Context, id string, fn func(Envelope, any) error) error {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"https://api.replicant.space/v1/events/stream", nil)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "text/event-stream")
	if id != "" {
		req.Header.Set("Last-Event-ID", id)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		log.Fatalf("unexpected status %s: %s", resp.Status, body)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		log.Fatalf("unexpected content-type %q", ct)
	}
	return readEvents(resp.Body, fn)
}
