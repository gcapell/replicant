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
| 2a | `travel` to the observatory, if the transport isn't already there | transport | ~10 min | wait (`auto.GoTo`) |
| 2 | `attach` (`targets: [obs]`) | transport at the same location | instant | do it |
| 3 | `travel` to the next star's entry point | transport | ~10 min | wait (`auto.GoTo`) |
| 4 | `detach` (`targets: [obs]`) | transport | instant | do it |
| 5 | `unfurl` | observatory | hours | start it and move on |
| 6 | `prospect` with `direction` | observatory | 12 h | start it and move on |

The program does every short step it can. Once each observatory is waiting on a
long step (or is stuck), it exits. Run it again later, e.g. from cron, to push
things on.

## Working out where an observatory is in the cycle

Nothing is saved between runs except the [ledger](#ledger). Where devices are
comes from the store (`~/.replicant/replicant.db`, `-db`), so `monitor` must be
running. Each observatory's status comes from `/devices/{code}`, and it's an
error (for the whole run, all problems listed) if the API and the store disagree
about its type, `location`, `attached_to` or `stowed_in`:

| Device | State | Next steps |
|---|---|---|
| status `compacting` / `unfurling` / `prospecting` | busy | none: long wait |
| `compacted`, attached, transport travelling | moving | wait for arrival, detach, unfurl |
| `compacted`, attached, at a star we can prospect from | arrived | detach, unfurl |
| `compacted`, attached, elsewhere | — | travel, detach, unfurl |
| `compacted`, not attached, at a star we can prospect from | — | unfurl |
| `compacted`, not attached, elsewhere | — | (fetch a transport), attach, travel, detach, unfurl |
| `idle`, at a star we can prospect from | unfurled | prospect |
| `idle`, elsewhere (already prospected here) | unfurled | compact |
| anything else | unknown | none: report it |

"A star we can prospect from" means a star that is in the candidate list (below),
isn't in the ledger, and isn't claimed by another observatory. A new observatory
that's already at such a star therefore prospects there without moving.

## Choosing the next star and direction

- **Candidates:** the store's `relay_stars` (stars with a `relaying` ftl_relay).
  The travel destination is the star's `entry_point` in the store's `stars`; it
  is *not* the relay's location, since a relay can sit elsewhere in the system
  (e.g. on a belt). A relay star whose entry point isn't known, or that isn't in
  `stars` at all, is skipped with a warning.
- **Centre:** the mean position of the candidate stars, from the store.
- **Order:** by distance from the centre, nearest first, as in `observatory_plan`, so the explored sphere fills in as densely as possible.
- **Excluded:** stars in the ledger (prospected from, or refused as too dense), and
  stars claimed by another observatory: the one it's sitting at, if it can prospect
  there, or its transport's travel destination.
- **Direction:** the star's position minus the centre, i.e. pointing straight out.
- Observatories are planned in device-code order. Each one takes the first
  unclaimed candidate.

## Choosing a transport

Transports come from the store: every device whose `features` include `surge`
and `attach` (devices with no `features` in the store can't be considered, and
their number is logged). Its attach capacity is its blueprint's, and it's
carrying something if any device's `attached_to` is it.

A transport is free if the store has it `idle`, at a location (not travelling or
stowed), not attached to anything, carrying nothing, and not already taken by
another observatory this run. Of the free ones, take, in order of preference:

1. one at the observatory's exact location;
2. one elsewhere in the observatory's system;
3. the one whose star is nearest the observatory's star (star distance; a
   transport at a star the store has no position for isn't considered).

Ties go to the smallest attach capacity, then the device code. A transport that
isn't at the observatory's location first travels there (the "fetch" step).

This is greedy: a free transport may be one that's waiting for a work crew it
dropped off (e.g. `auto_salvage`'s E62C8DC1), and taking it
leaves the crew to be picked up some other way. Sharing transports between
crews properly is for later.

**Checking with the API:** the transports were chosen from the store's statuses,
so before anything is sent (and with `-n` too) every transport a plan uses is
fetched from `/devices/{code}`. It's an error, for the whole run with all
problems listed, if the API disagrees with the store about its type, location,
`attached_to`, `stowed_in` or status, or (if travelling) its final destination,
or if one that's to pick up an observatory is already carrying something.

After `detach` the transport stays at the new star, ready for that observatory's
next cycle (or anyone else's).

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
- **fetch / travel:** `auto.GoTo` on the transport; a trip already under way
  uses `auto.FinishTrip`.
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

Both modes are implemented. Only `-n` has been run against the live API, and
not since it moved to the store; the actions, including fetching a transport,
haven't been tried yet. `-n` reads from the API (statuses), but sends no
commands. Once `monitor` tracks prospecting starting and finishing, the
observatories' statuses could come from the store too.
