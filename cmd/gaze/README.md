# gaze

Cycles our `galactic_observatory` devices through observation points, prospecting
outward from the centre of explored space.

```bash
go run ./cmd/gaze -n            # dry run: show each observatory's state and next steps
go run ./cmd/gaze               # do them
```

Background: <https://replicant.space/docs/galactic-observatory/>. An observatory
only finds stars where catalogued space is sparse, and each star can be prospected
from only once.

## The cycle

For each observatory, repeat:

| # | Step | Sent to | Takes | We… |
|---|---|---|---|---|
| 1 | `compact` | observatory | hours | start it and move on |
| 2 | `attach` (`targets: [obs]`) | transport at the same location | instant | do it |
| 3 | `travel` to the next star's entry point | transport | ~10 min | wait (`auto.GoTo`) |
| 4 | `detach` (`targets: [obs]`) | transport | instant | do it |
| 5 | `unfurl` | observatory | hours | start it and move on |
| 6 | `prospect` with `direction` | observatory | 12 h | start it and move on |

The program does every short step it can. Once each observatory is waiting on a
long step (or is stuck), it exits. Run it again later, e.g. from cron, to push
things on.

## Working out where an observatory is in the cycle

Nothing is saved between runs except the [ledger](#ledger). State comes from the
device each time:

| Device | State | Next steps |
|---|---|---|
| status `compacting` / `unfurling` / `prospecting` | busy | none: long wait |
| `compacted`, attached, transport travelling | moving | wait for arrival, detach, unfurl |
| `compacted`, attached, at a star we can prospect from | arrived | detach, unfurl |
| `compacted`, attached, elsewhere | — | travel, detach, unfurl |
| `compacted`, not attached, at a star we can prospect from | — | unfurl |
| `compacted`, not attached, elsewhere | — | attach, travel, detach, unfurl |
| `idle`, at a star we can prospect from | unfurled | prospect |
| `idle`, elsewhere (already prospected here) | unfurled | compact |
| anything else | unknown | none: report it |

"A star we can prospect from" means a star that is in the candidate list (below),
isn't in the ledger, and isn't claimed by another observatory. A new observatory
that's already at such a star therefore prospects there without moving.

## Choosing the next star and direction

- **Candidates:** stars with one of our ftl_relays that isn't stowed. The relay sits at the
  system's entry point, so its location is the travel destination, and there's no
  need for a `system_scan`.
- **Centre:** the mean position of the candidate stars. Positions come from
  `/replicants/4698E8E6/stars?per_page=N` (`-stars`, default 50). Relay stars
  that aren't in that list are ignored, with a warning.
- **Order:** by distance from the centre, nearest first, as in `observatory_plan`, so the explored sphere fills in as densely as possible.
- **Excluded:** stars in the ledger (prospected from, or refused as too dense), and
  stars claimed by another observatory: the one it's sitting at, if it can prospect
  there, or its transport's travel destination.
- **Direction:** the star's position minus the centre, i.e. pointing straight out.
- Observatories are planned in device-code order. Each one takes the first
  unclaimed candidate.

## Choosing a transport

Fetching: we never list every device. We fetch the observatories and ftl_relays
by `device_type`, then `/devices?location=LOC` for each observatory's location,
plus the transport of any observatory that's attached (by code, since it may be
travelling). Candidate transports are those at the observatory's exact location, with features `surge` and `attach`, status
`idle`, and `attach_used < attach_capacity`. Of those, take the smallest
`attach_capacity`. Each transport is used by at most one observatory per run. If
none is free, report it and skip that observatory. After `detach` the transport
stays at the new star, ready for that observatory's next cycle.

## Ledger

`$HOME/observed_from.txt` lists the stars we've already observed from, one per
line. Anything after the star name is a note. Blank lines and `#` comments are
ignored. A missing file means nothing has been observed yet.

```
# seeded by hand
MIRETH
SYLSHENOS dense ratio=3.2 outward_ratio=2.9
```

- A star is added when a `prospect` command is accepted, or when the API says the
  star has already been surveyed from.
- A star is also added, noted `dense …` with the `ratio`/`outward_ratio` reported,
  when `prospect` is refused with HTTP 400 (too many neighbours). The observatory
  then compacts and moves on to the next star.
- Edit it by hand to seed stars we observed from before `gaze` existed.

## Doing the steps

Each observatory's steps run in their own goroutine. The hub is only opened if
something travels.

- **attach / detach:** sent to the transport with `targets: [obs]`. We then fetch
  the observatory to check `attached_to_device_code`. After detach we also check
  it's in the destination system before unfurling, since unfurling takes hours.
- **travel:** `auto.GoTo` on the transport; a trip already under way uses
  `auto.FinishTrip`.
- **compact / unfurl:** we send it, then fetch the device and log its status.
- **prospect:** sent with `direction`. On success the star is added to the
  ledger (`prospected <time>`). A 400 whose body reports `ratio`s is recorded as
  `dense ratio=… outward_ratio=…`. One that says "already" or "surveyed" is
  recorded as `already surveyed (refused)`. Either way the observatory then
  compacts, so the next run moves it on. Any other error is a failure, and
  nothing is recorded.

## Failures

A failed step is logged and stops that observatory for this run. The others carry
on. The program exits non-zero if any observatory failed.

## Assumptions not yet checked against the live API

- The status of an unfurled, unoccupied observatory is `idle`.
- An attached observatory's `location` follows its transport.
- Transports report `attach_capacity` / `attach_used` in `/devices`.
- `prospect` takes `direction: [x, y, z]` in the same frame as `/stars` positions.
- The "already surveyed" and "too dense" refusals can be told apart from the
  error body.
- Observatories have their own `travel`/`cruise`, but cruise only works within a
  system, so we use a surge transport between stars.

## Status

Both modes are implemented. Only `-n` has been run against the live API; the
actions haven't been tried yet.
