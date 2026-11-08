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

## TODO before running it for real

`monitor` doesn't yet apply anything plant_relays does, so after a run the
store would still show the old state, and the next plan (and `verify`) would
be wrong. Not handled yet:

- `change_owner` (the relay's `owner`);
- `stow` (`stowed_in`, `location`, status `stowed`);
- `deploy` (`stowed_in` cleared, `location` = the host's);
- `activate` (status `relaying`, which is what `relay_stars` counts);
- `system_scan` (the star's `entry_point`, and its system).

The plan is to run plant_relays once, see which events these produce in
`events`, teach `monitor` to apply them, and replay them with
`monitor -catchup EVENT`.

**Caveat:** none of these actions shows up in the 2026-11-08 archive or the
live store so far (the archive has no stow, deploy, activate, change_owner or
scan events), so they may not produce events at all. If they don't, `monitor`
will need another way to learn about them: for example, re-reading the devices
and the star after a run, or polling.
