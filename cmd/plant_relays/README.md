# plant_relays

```bash
plant_relays [-n] [-db PATH]
```

Plants ftl_relays at every star that can join the relay mesh. See the doc
comment in `plant_relays.go` for how it plans; this file covers where its data
comes from and what still needs doing.

## Planning from the store

The plan comes entirely from the store (`load.go`), so `-n` never calls the
API.

- **Stars:** every star in `stars`. Those with a `relaying` ftl_relay are the
  mesh; the rest are candidates. A star with no `entry_point` counts as
  unscanned (shown as `(unscanned)` in the plan), and the vessel goes to its
  Oort cloud and runs `system_scan` first. A relay star that isn't in `stars`
  is an error.
- **Vessels:** heaven_vessels that are idle, not attached, not stowed, and have
  a `replicant_matrix` stowed in them (`auto.ReadyReplicant`). The replicant is
  the vessel's `owner`. Free slots are the heaven_vessel blueprint's
  `stow_capacity` minus the number of devices with `stowed_in` = the vessel.
- **Spare relays:** ftl_relays at a `-BELT-` location, not stowed, not
  `relaying`, with their `owner`.

## Before a real run

`verify.go` checks the plan against the API and stops, reporting every
difference, if:

- the API's set of relay stars differs from the store's;
- a vessel with a route isn't ready, or its location, replicant, stowed relays
  or free slots differ;
- a relay it will fetch isn't where the store says, or is stowed, attached,
  `relaying`, or owned by someone else.

## What monitor knows about a run

`monitor` applies `device.deployed` and `relay.activated`, so after a run the
store has the planted relays as `relaying` and the next plan is accurate. To
apply events recorded before monitor handled them (in the order they
arrived, so each deploy comes before its activation):

```bash
go run ./cmd/monitor -catchup device.deployed,relay.activated,experience.gained
```

Still not applied:

- `stow` and `change_owner`, used when a vessel fetches relays from a belt.
  No run has fetched yet, so we haven't seen whether they produce events
  (`device.stowed` and `device.changed_owner` are registered in
  `model/sse.go`). Until they're handled, `verify` will catch the stale
  store and refuse to start a run that fetches.

A `system_scan` produces only `experience.gained` (`"source":"scan"`, the star
as `location`), so monitor reads `/locations/STAR` for any system it hasn't
read yet, which gives the star its entry point, planets and belts.
