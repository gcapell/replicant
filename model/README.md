# model

Response structs (`model.go`, `device.go`) and SSE event payloads (`sse.go`). See the top-level [README](../README.md) for how JSON field names are derived.

## Times in SSE events

Most times in event payloads carry an offset (`2026-11-08T17:28:15+11:00`, `...Z`), but some don't: `device.compacting`'s `completes_at` is `"2026-11-09T00:21:43.987437"`. **A time with no offset is UTC.** (That one came 8640s, 2h24m, after the event's `created_at`; read as local time it would already have passed.)

Parse event times with `model.ParseEventTime`, which takes RFC 3339 or a bare UTC time.
