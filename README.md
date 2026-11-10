# replicant

Go automation tools and an Angular dashboard for the [replicant.space](http://replicant.space/) game API.

- API docs: <http://replicant.space/docs/> (each endpoint has its own page, e.g. `/docs/api/devices/command/`)
- Base URL: `https://api.replicant.space/v1`, with `Authorization: Bearer $API_KEY`

This file records how the code is put together and the conventions we've settled on, so that a new piece of work can start from here.

## Layout

| Path | What |
|---|---|
| `api/` | Thin HTTP client: one small function per endpoint or command, plus the SSE event hub. |
| `model/` | Response structs (`model.go`, `device.go`) and SSE event payloads (`sse.go`). Its [README](model/README.md) covers times in events: **one with no offset is UTC** (`model.ParseEventTime`). |
| `auto/` | Shared higher-level helpers used by the commands (device listing, travel-and-wait, location tests). |
| `store/` | The local SQLite database (see [The store](#the-store-store)). |
| `cmd/<name>/` | One `main` package per tool. |
| `ui/` | Angular dashboard (see [UI](#ui)). |
| `design/` | Design notes for work not built yet. [tasks.md](design/tasks.md) plans a task table and turning `monitor` into an executor that carries tasks out. |

**Showing resources:** abbreviate them as C (carbon), Co (conductive), R (rares), Si (silicates), S (structural) and V (volatiles), and always list them in alphabetical order of the full name: `C=low Co=high R=scarce Si=scarce S=moderate V=scarce`. In Go, `model.Resources` is that order and `ResourceType.Abbrev()` gives the short name.

`api` stays thin on purpose. Anything that combines calls or makes a decision goes in `auto` (if more than one command could use it) or in the command itself.

## Building and testing

```bash
go build ./... && go vet ./...
API_KEY=x go test -race ./...
```

- `api`'s `init` exits if `$API_KEY` is unset, so tests need a dummy key. Tests never call the real API.
- Go 1.26: `wg.Go`, `errors.Join`, `min`/`max`, range-over-func and `for i := range n` are all fair game.
- Keep the testable part of a command a pure function (e.g. "choose which relays/sites/stars") and test that. The API-calling glue stays small.
- Run `gofmt` before finishing.

## Talking to the API (`api/`)

- **JSON field names** are derived, not tagged: jsoniter with `extra.LowerCaseWithUnderscores`. `CargoCapacity` becomes `cargo_capacity`.
  - The strategy puts `_` before *every* capital letter, so acronyms and fused words break: `AMIDirective` → `a_m_i_directive`, and `Amidirective` → `amidirective` (neither matches `ami_directive`). Use `AmiDirective`, or add an explicit `json:"..."` tag.
  - When adding a struct, check every field name against a real response. A misnamed field decodes silently as the zero value.
- **Paths go through `fmt.Sprintf`** in `api.do`, so a path built from data must not contain `%`. Build query strings by concatenation, not `url.QueryEscape`.
- **Reading:** `api.Get(ctx, new(T), "/path/%s", id)`.
- **Paging:**
  - Integer cursors: `for page, err := range api.Paged[model.DeviceList](ctx, "/devices?...")`. The model type implements `model.Pageable`.
  - `/inventory` uses a *string* cursor (a location code), so `api.InventoryAt` has its own loop. `InventoryAt(ctx, "")` lists everything, `InventoryAt(ctx, "STAR")` one system, `InventoryAt(ctx, "STAR-3")` one location.
- **Device commands:** `POST /v1/devices/{code}` with `{"command": ..., ...args}`, via the unexported `deviceCommand`. Existing wrappers:
  - travel and cargo: `Travel`, `CollectResources`, `DepositAllResources`, `DepositResources` (just the given amounts);
  - attaching and AMI: `AttachDevice`, `DetachDevice`, `AdoptDevice`, `Salvage`, `SurveySystem` (`survey_system` directive, all planets and moons, no recall);
  - maintenance: `Patrol` (`patrol` directive), `ClearDirective`;
  - belt directives: `GatherEvenly` (`gather_evenly`, for an ami_mining_controller), `BeltSearch` (`belt_search`, for an ami_survey_controller with belt_surveyors);
  - ownership and stowing: `ChangeOwner`, `Stow`, `Deploy`;
  - relays and scanning: `Activate`, `SystemScan` (returns the system's `entry_point`);
  - modular devices and observatories: `Compact`, `Unfurl`, `Prospect` (takes a `direction`).
- **Other POSTs:** `CompleteEvent(ctx, loc, designation)` (empty body).
- **Errors:** a non-2xx response is an `*api.HTTPError`; use `errors.As` to read its `StatusCode` and `Body`.
- **Autofactory** (`/devices/{code}`): `printing` is `{completes_at, device_type, eta_seconds, progress_percent, started_at, tags}`, and `print_queue` is a list of `{device_type, notify: {device}}` with no quantity or time, one entry per device (five mining_drones show as five entries; presumably an enqueued `quantity` expands the same way). `queue_size` is the queue's capacity (10). What `printing` looks like on an idle factory hasn't been seen yet. A real response is in `api/device_test.go`.
- **AMI directive status** (`ami_directive_status` on `/devices/{code}`): `active`, `completed` and `paused` seen so far. A controller whose `survey_system` had completed showed it `paused` after being detached from its carrier by hand; no `directive.paused` event arrived. Task handlers treat anything but `active` as not running.
- **Galactic observatory** status values seen so far: `compacted`, `compacting`. Its features include `cruise` (in-system only) and `modular`. A compacted observatory needs a transport with `surge` and `attach` to move between systems.
- **Every request is logged** (`GET /path`, or `POST /path {body}`).

### API facts we've confirmed

- **Device status** values include `idle`, `travelling`, `stowed`, `relaying`, `coordinating`, `monitoring`, `searching`, `warding`, `scanning`, `unfurling`, `tracking`, `out_of_range` and `inactive` (an FTL relay that isn't relaying). Some carry an argument: `mining (rares)`, `printing (survey_drone)`, `repairing (5A09A829)`.
- **`/devices` (the list)** includes `stowed_devices`, `travel` and `hosting_replicant` (only on a vessel carrying a replicant), so there's no need to read devices one by one for those. A stowed or travelling device has a null `location`. Times carry an offset (`2026-11-08T17:28:15+11:00`).
- **Moving devices:**
  - `stow` is sent to the *device being stowed*: `{"command":"stow","target":<host device>}`. It is immediate: no error means it worked.
  - `change_owner` is sent to the device: `{"command":"change_owner","target":<replicant_code>}`. Both replicants must be on the same account. A device must belong to a vessel's replicant before it can be stowed in that vessel.
  - `deploy` is sent to the *stowed device*, with no arguments (as the UI's deploy button does). It's immediate: fetch the device straight afterwards and its `location` is the host's.
  - `activate` is sent to the deployed ftl_relay and takes effect at once: its status should be `relaying` straight afterwards.
  - `attach` and `detach` are sent to the carrier with `targets: [...]`. The two devices must be at the same location.
- **FTL relays** are normally either `relaying` at an `-L4` point, `idle` at a `-BELT-` location where they were just made (the spares), or stowed in a heaven_vessel. Only `relaying` ones count towards the mesh. heaven_vessels can travel outside the mesh.
- **Travel and cargo:**
  - `travel` takes just a `destination`. Surge-capable devices (e.g. cargo_freighter) work out their own route.
  - `collect_resources` and `deposit_resources` are synchronous; we still look for the confirming event (see the hub section).
  - `deposit_resources` takes an optional `resources` map (`{"silicates": 50}`); without it the whole hold is unloaded.
- **Inventory:** `/inventory` `items` is a **list** of `{resource_type, quantity}`, although the docs show a map. Handle both in the UI.
- **Locations:**
  - `/v1/locations` returns a map of location code to `{devices, location_events, replicants, resource_sites, resources}`. Moons' salvage sites only appear here, not under `/locations/PLANET`.
  - `/v1/devices?location=X` takes either a star (`MIRETH`) or an exact location (`MIRETH-4-L4`).
  - `/v1/locations/PLANET` (`api.Location`) has a `location_event` (`model.LocationEvent`) when there's an event there. Its `progress.options` are alternative ways to meet it, each with `resources` and `devices` lists of `{…, current, required, met}`. An event is completed with `POST /v1/locations/{loc}/events/{designation}`.
  - `/v1/locations/STAR` only answers for a system we've scanned (`system_scan`, which planting a relay includes). Otherwise it's an error with body `{"error": "No replicant in system"}`, despite the wording. Of our 60 nearest stars, the 42 it answered for were exactly the relay stars. Its `asteroid_belt.belts` list each belt's `density` and resource levels; each planet has an `inventory` list like `/inventory`'s.
  - `/v1/locations/STAR` (`api.Location`) gives the system: `system_scanned`, `planets_scanned`/`planets_total`, `moons_scanned`/`moons_total` (`Location.Unscanned()` sums what's left), `entry_point`, `planets`, and `star.position`.
- **Relays:** only send devices to a system whose ftl_relay is `relaying`. Anything sent to a system without an active relay can't be controlled until a replicant goes there to rescue it.
- **System scan:** `{"command":"system_scan"}` sent to a vessel scans the system it's in. This is what `/replicants/{code}/scan` runs. The response's `entry_point` (e.g. `CORBRENEUS-5-L4`) is the system's easiest-to-reach L4 point, which is where we put relays. `/stars` also returns `entry_point` for stars we already know it for.
- **Stars:**
  - `/v1/replicants/{code}/stars?per_page=30` gives the nearest stars, with positions (`api.Stars(ctx, code, perPage)`). Our replicant is `4698E8E6`. `per_page` is at most 50; later pages come from `&page=N`, counting from 1.
  - `/v1/replicants/{code}/stars/{star}` (`api.StarNear`) gives one star, as `{"replicant_position": ..., "star": {...}}`; the star also has `spectral_type`, `explored` and `has_life`.
  - **Never fetch `/v1/stars`.** That's the whole catalogue, around a million stars, and it's rate limited.
- **Events:**
  - `/v1/accounts/events?status=active` (integer cursor, `model.AccountEvents`) lists every event with its full `progress`, so there's no need to scan `/locations`. Each event has `location`, `designation`, `status` and `progress.options[]`, and each option has `resources` and `devices` lists of `{…, current, required, met}`. `criteria` repeats the requirements without progress.
  - An option's `met` (and `progress.met`) stays **false until a replicant is present** (`progress.replicant_present`), even when every item in it is met. Use `EventOption.Satisfied()` to ask whether everything's there.
  - Complete an event with `POST /v1/locations/{loc}/events/{designation}`, empty body, with a replicant (a heaven_vessel carrying a `replicant_matrix`) at `loc`.
- **Blueprints:** `/v1/blueprints` (`api.AllBlueprints`, not paged) gives every device type's print cost (`resources`), `components` (devices consumed, e.g. a processing_array takes 5 compute_cores), `print_time` (seconds), `features`, `directives`, capacities (`attach`, `cargo`, `stow`, `queue_size`), `relay_range` (ly: ftl_relay 7.5, system_hub 15) and `strength`. system_hub's also has `current_hubs`, which we ignore. A real response is in `store/testdata/blueprints.json`.
- **Accounts:** `/v1/accounts/reputation` returns `{"reputation": [{name, species_key, description, trait, total_reputation}]}`.

### Location codes

| Pattern | Meaning |
|---|---|
| `STAR-N` | planet |
| `STAR-N-M` | moon |
| `STAR-N-L4` / `-L5` | Lagrange point (where FTL relays etc. must be activated) |
| `STAR-BELT-N` | asteroid belt |
| `STAR-OBJ-…` | asteroid on its own. **Never send miners or freighters there**: it can collide with a planet. |

- `auto.StarOf(loc)` returns the star part of a code.
- `auto.IsPlanetOrMoon(star, loc)` matches planets and moons only. Use it whenever a command should take from "planets and moons".

## Waiting for results: the event hub (`api/hub.go`)

There is one SSE connection per process (`/v1/events/stream`). Goroutines register one-shot waiters with it.

```go
hub := api.NewHub(ctx)

// 1. Register BEFORE acting, so a fast event isn't missed.
w := hub.Expect(api.AnyOf(
	api.ForDevice[model.DirectiveCompleted](controller),
	api.ForDevice[model.DirectiveCleared](controller),
))
// 2. Act.
if err := api.Salvage(ctx, controller, site); err != nil {
	w.Cancel() // always cancel a waiter you won't Wait on
	return err
}
// 3. Wait (bound it with a context timeout where appropriate).
ev, err := w.Wait(ctx)
switch p := ev.Payload.(type) { case *model.DirectiveCompleted: ... }
```

How the hub behaves:

- **Waiters:** each fires at most once. A match function sees `api.Event{Env, Payload}`, where `Env.DeviceCode`, `Env.Location` and `Env.Star` identify what the event is about.
- **Payloads** are pointers to the type registered in `model/sse.go` (`register[T]("event.name")`). If you need a new event, add its struct and registration there. An undecodable event arrives with a nil payload and is skipped by type-matching waiters.
- **Resuming:** the hub saves the last event ID to `/tmp/replicant/eventID.txt` and resumes from it. Events created more than 30s before a waiter registered never wake it, so replayed history is ignored.
- **Reconnecting:** if the stream drops, the hub reconnects with backoff (1s, doubling up to 1 minute).
  - Known gap: it retries forever, even on a 401.
  - Known gap: there's no idle timeout, so a half-open connection after the laptop sleeps can hang it.
- **Events we use:**
  - directives: `directive.completed`, `directive.cleared`;
  - AMI progress: `ami.survey.digest` (`Report.Progress` has scanned/total/remaining);
  - travel: `travel.departed` (has `TravelTimeSeconds`, used for the ETA; `arrives_at` per the docs on single-leg trips; `destination` is the final one; `attached_devices` when carrying), `travel.arrived` (sent only at the end of a trip, not for intermediate legs, so any arrival for the device ends the wait. A surge to another system reports the bare star as `destination`, e.g. `"PARBEA"`, with the real place in the event's `Location`, e.g. `PARBEA-OORT`), `travel.cancelled`;
  - cargo: `transport.collected`, `transport.delivered`.
- **Synchronous commands** (collect, deposit) treat a missing confirmation as a warning, after a 10s timeout. Long waits (travel) use 12h.

### Travel helpers (`auto/travel.go`)

- `auto.GoTo(ctx, hub, device, loc)`: send and wait for arrival. If the device is already there it does nothing; if it's already heading there it just waits. It logs `X travelling A -> B (ETA 3m22s)`.
- `auto.GoToAll`: run `GoTo` for several devices concurrently.
- `auto.FinishTrip`: wait out whatever trip a device is on.
- `auto.ShortRoute(start, points, pos)`: a short open path through points (nearest neighbour, then 2-opt), with `auto.RouteLength`.
- Device listing helpers: `auto.Devices(ctx, type)`, `auto.Everything(ctx, loc)` (all types), `auto.DevicesInSystem(ctx, star, type)` and `auto.DevicesAt(ctx, loc, type)`.
- `auto.RelayStars(ctx)`: the stars with a `relaying` (not stowed) ftl_relay.
- `auto.StowInto(ctx, device, owner, host, hostReplicant)`: `change_owner` to the host's replicant if needed, then `stow`. Any device stowed in a heaven_vessel must belong to its replicant first.
- `auto.ReadyReplicant(d)`: an idle, unattached, unstowed heaven_vessel carrying a `replicant_matrix`.

### Splitting work among devices (`auto/spread.go`)

These are generic over the item type, taking a position function as `ShortRoute` does. Used by `auto_scan` and `complete_events`.

- `auto.Seeds(items, k, pos)`: k spread-out items, maximising the minimum distance between them (exhaustive up to 200k combinations, then farthest-first).
- `auto.GroupNearest(items, seeds, pos)`: each item joins its nearest seed's group.
- `auto.Assign(agents, apos, groups, pos)`: a different agent for each group, minimising the total distance from each agent to its group (exhaustive with pruning, so keep the numbers small).
- `auto.Order(start, group, pos)`: the nearest item first, then `ShortRoute`.
- `auto.DistToSet(p, set, pos)`.
- `auto.MatchMinMax(nAgents, nTasks, dist)`: pairs agents with tasks, one each; a `+Inf` distance is an impossible pair. It makes as many pairs as it can, then minimises the longest distance, then the total. Exhaustive with pruning. Used by `monitor` for survey and move tasks.

## The store (`store/`)

A SQLite database (`modernc.org/sqlite`, pure Go) at `~/.replicant/replicant.db` (`store.DefaultPath()`), where we remember what we've learnt from the API so we don't have to ask again. Several commands use it at once, so `store.Open` sets `busy_timeout`, WAL and `_txlock=immediate` (transactions take the write lock when they begin).

### One writer (`monitor`), many readers

The aim is for the store to be an up-to-date picture of the game, so that commands can look things up there instead of asking the API.

- **`monitor` is the only program that writes to the store.**
  - **Startup:** it takes a snapshot from the API: devices, the nearest stars and their systems, inventory and blueprints.
  - **Events:** it then follows the event stream, recording every event in `events` and applying the ones it understands to the other tables, each in one transaction.
  - **Leave it running** (`caffeinate -i go run ./cmd/monitor`) whenever other commands rely on the store. When restarted, it resumes the stream from the last recorded event, so nothing is missed (see its row under [Commands](#commands)).
- **Every other command only reads.** Open the store read-only (`PRAGMA query_only=1`, see below). Never `INSERT`, `UPDATE` or call the `store.Put…` helpers from anywhere but `monitor`.
  - To keep something new in the store, teach `monitor` to fetch it, or to apply the event that reports it.
  - Commands still *act* through the API. `monitor` sees the resulting events and updates the store.
- **Read the store for facts that change slowly:** star positions, entry points, systems' planets and belts, which stars have relays, and where idle devices are. Ask the API for anything that must be exactly current before acting, such as a device's status just before sending it a command, or the inventory being collected.
- **The store is only as complete as `monitor` makes it.** Events it doesn't apply yet (cargo, stowing, `directive.paused`/`resumed`) leave some columns stale until the next fresh snapshot. The [applied events](#applied-events) list says what's covered. Starting from a fresh snapshot means deleting `~/.replicant/replicant.db*` and restarting `monitor`.
- **Opening the store:** `store.Open` migrates the schema, so only `monitor` and `migrate_db` should use it. Every other command uses `store.OpenReadOnly` (`query_only`, no migration; an error if the file doesn't exist), e.g. `start_belt`, `print_queue`, `plant_relays`, `gaze` and `complete_events`.

### Schema and tables

- **The schema is `store/schema.sql`.** To change it, edit the file. `store.Open` runs it in an empty in-memory database, compares that with the real one's `sqlite_master`, and makes them match (`store.Migrate`), in one `BEGIN IMMEDIATE` transaction:
  - a new table is created;
  - a table whose `CREATE` differs (ignoring comments and whitespace) is rebuilt: a new table is created, the shared columns are copied across, the old table is dropped and the new one renamed. An `AUTOINCREMENT` table keeps its counter (copying would otherwise reset it to the highest id left, and `tasks` ids would then clash with `archived_tasks`);
  - indexes, views and triggers that differ are dropped and recreated.
- **Dropping a table or column loses data**, so `Open` refuses to. Run `go run ./cmd/migrate_db -allow-drops` to allow it.
- **Renaming** a table or column looks like a drop plus an add, so its data is lost.
- **A new `NOT NULL` column needs a `DEFAULT`**, or rebuilding a table with rows fails (and the migration changes nothing).
- Tables are `STRICT`: a value that can't be converted losslessly to a column's type is an error. Booleans are `INTEGER` 0/1. Times are `INTEGER` Unix milliseconds (`time.UnixMilli`); read one with `datetime(t / 1000.0, 'unixepoch', 'subsec')`.
- **`events`** records every event received from the stream: the envelope's fields as columns, and `payload` as JSON. An event should be recorded in the same transaction that applies it, so the last row (highest `seq`) is where to resume the stream. Event IDs (`1794115235797-0`) don't sort as text, which is why there's `seq`. There are no per-row timestamps elsewhere.
- **`stars`:** position and `entry_point` (from `/replicants/{code}/stars`), then, from `/locations/STAR`: the planet and moon counts, `is_fully_scanned` (generated, `STORED`), `life_detected`, `mining_bonus_pct` and `system_tags` (comma-separated). Those are NULL until we've read the system.
- **`locations`:** planets from `/locations/STAR` (`type`, `in_habitable_zone`, `life_stage`, `moon_count`, `scanned`). Moons and other places can join later.
- **`belts`:** each belt's `density` and one column per resource holding its level (`low`, `moderate`, `high`).
- **`inventory`:** resources lying at a location (planets, moons and belts), one column per resource, from `/inventory` only. A location holding nothing has no row.
- `locations`, `belts` and `inventory` have a generated `star` column, like `devices`.
- **`blueprints`:** one row per `device_type` (the key) from `/blueprints`. `features` and `directives` are comma-separated, `components` is JSON (`{"compute_core":5}`; read it with `json_each`), and the resource columns are the cost to print one. Empty lists are NULL.
- **`devices`:** `owner` is the replicant code. `features` is comma-separated, like `system_tags`; rows written before it existed have it NULL until devices are fetched again. `completes_at` is when a compact or unfurl finishes, set only from those events (`PutDevice` leaves it alone). `star` is a generated `STORED` column (the part of `location` before the first `-`; NULL if `location` is).
- **`relay_stars`** (a view): stars with a `relaying` ftl_relay, worked out from `devices`.
- **Task tables** (`tasks`, `blockers`, `claimed_resources`, `claimed_devices`, `adopted_maintenance_drones`, `archived_tasks`): for the planned executor; see [design/tasks.md](design/tasks.md). So far only `monitor`'s startup writes `tasks` (`survey_system` tasks, see its row under [Commands](#commands)), and `adopted_maintenance_drones` is filled by hand. Nothing carries tasks out yet. Each task has a `status` (pending, blocked, assigned, failed) and a `phase`: where within the task it is, set by its handler (`store.SetPhase`, assigned tasks only; NULL until started). `move_devices`: `to_pickup`, `to_destination`. `survey_system`: `gathering` (at the entry point of the system it's in, to be picked up), `surveying`, `regrouping` (at the target's entry point). A handler that only waits also fills in an obvious phase (a travelling transport that's carrying something is `to_destination`). Helpers: `store.TasksOfType`, `store.AssignedTasks`, `store.PendingTasks`, `store.FinishTask` (archives as done and unblocks tasks left with no blockers), `store.Block`, `store.EntryPoint`, `store.IsFullyScanned`, `store.ControlledBy`, `store.FreeTransports`, `store.AttachCapacity`, `store.CheckRelayStar`, `store.AddTask`, `store.AssignTask` (pending and unassigned only), `store.UnsurveyedRelayStars` (an unread relay star is an error) and `store.FreeSurveyTeams`.
- <a id="applied-events"></a>**Events applied by `monitor`** (guesses marked; check them against the API):
  - A device's status on travel depends on `can_carry` (a generated column: a `heaven_vessel`, or `attach` in its `features`).
  - `travel.departed`: the device becomes `travelling` with no `location`, `destination` from the payload (always the final one), and `arrives_at` from the payload (present on every departure seen, multi-leg too; otherwise `created_at + travel_time_seconds`). A device that can't carry keeps its old status in `status_before_travel` (an AMI drone's or controller's status comes from its directive: `mining (carbon)`, `coordinating`, ...). Devices in `attached_devices` lose their `location`; their status is left alone (the API shows carried devices as `idle` or `coordinating`).
  - `travel.arrived`: the device's `location` is the envelope's `location` (after a surge the payload's `destination` is only the star). A device that can carry becomes `idle`; any other gets `status_before_travel` back, or `idle` if we didn't see it depart. `attached_devices` get the same location.
  - `print.completed`: fetches `/devices/{new_device_code}` and stores it (`PutDevice`), as the API describes it now. The payload has `new_device_code`, `device_type`, `print_mode` (`autofactory` so far) and `compacted`; the envelope's device is the printer. A device the API doesn't know, or of a different type, is an error.
  - `device.attached` / `device.detached`: one event per attached device. The envelope's device is the carrier and its `location` is where both are; the payload has `target_code` and `target_type`. The target's `attached_to` becomes the carrier (or NULL) and its `location` the envelope's; its status is left alone. Repeating an attach to the same carrier, or a detach, is fine (the snapshot may be newer than the event). It's an error if the target is attached to a different carrier, if `target_type` doesn't match the store, or if the carrier or target isn't in the store. The 160 attach and detach events in the archive replay without any of these.
  - `ami.mining.digest`: the envelope's device is the ami_mining_controller, and `devices` lists its devices with their status (`idle`, `travelling` or `mining (RESOURCE)`; anything else is an error). Each listed device gets that `status` and `controller` = the controller. While travel events have a device travelling (it has a `destination`), the status goes into `status_before_travel` instead, for arrival to restore (a digest's `travelling` never does). A digest's `travelling` doesn't touch `location`: AMI drones' own trips (e.g. to a salvage site under `gather_salvage`) mostly don't reach the stream as travel events. In the archive, 44 drones appear in digests but only 12 travel events are about any of them. So a drone's `location` can be stale while its controller moves it.
  - `ami.survey.digest`: as `ami.mining.digest`, for an ami_survey_controller. Its devices' statuses are `idle`, `scanning`, `searching`, `tracking` or `travelling`. Its `report.progress` (`scanned`, `remaining`, `total`) counts planets and moons together, and `total` grows as scanning planets turns up their moons (ULSIOS went 7, 9, 31, 61, 65), finishing at `planet_count + moon_count` as `/locations/STAR` gives them. So it can't be split into planets and moons; `store.SurveyProgress` only marks the envelope's star fully scanned (both scanned counts set to the totals) once `remaining` is 0 and `scanned` equals planets plus moons. A total of 0 (belt searches) is ignored. It's an error if the numbers don't add up, if the total is more than the planets and moons we know of, or if the star's system hasn't been read.
  - `ami.adopted` / `ami.released`: the envelope's device is the controller, and `devices` lists `{device_code, device_type}`. Each device's `controller` becomes the controller (or NULL). A type that doesn't match the store is an error, as is a release by a controller other than the one the store has.
  - `directive.set`, `directive.completed`, `directive.cleared`: none says what the device's status becomes (an ami controller goes `coordinating`, for example), so `monitor` fetches `/devices/{code}` and takes just its `status` (kept for arrival while travelling, as for digests). When replaying, that's the status now, not then. A device the API doesn't know, or of another type, is an error. When an ami_survey_controller completes `survey_system`, `monitor` also reads `/locations/STAR` (the envelope's star) again and stores it (`store.PutSystem`), as a check on the digests; if planets or moons are still unscanned it logs that, and the `survey_system` task will start another survey.
  - `device.compacting` / `device.unfurling`: status `compacting` / `unfurling` and `completes_at` from the payload (an offset-less time is UTC). A missing or unparseable time is an error.
  - `device.compacted`: status `compacted`, `completes_at` NULL. `device.unfurled`: the status is fetched from `/devices/{code}` (we haven't seen what it becomes), `completes_at` NULL. For all four the envelope's location is ignored, and a device not in the store, or whose type doesn't match the envelope's, is an error.
  - `device.deployed`: the envelope's device has been deployed at the envelope's `location` from the payload's `deployed_from` (the vessel it was stowed in). Its `stowed_in` is cleared and its `location` set; the event doesn't give its status, so `monitor` fetches `/devices/{code}` (as for directives; when replaying that's the status now, e.g. `relaying`). Being already unstowed at that location is fine; being stowed elsewhere, at another location, or of another type is an error.
  - `relay.activated`: the payload has `location` and `star`, which must match the envelope. The ftl_relay becomes `relaying` (so `relay_stars` includes it). A device that isn't an ftl_relay, is stowed, or is somewhere else in the store is an error.
  - `experience.gained`: only `"source":"scan"` matters, with the bare star as the envelope's `location` (anything else is an error). A vessel's `system_scan` produces one (500 XP), but so does each AMI planet or moon scan (10 or 100 XP), so the amount isn't relied on. If the store hasn't read that star's system (`planet_count` is NULL, or the star isn't there), `monitor` reads `/locations/STAR` and stores it (`store.PutSystem`: entry point, counts, planets, belts). Otherwise it does nothing, so a survey costs no API calls. The new planets' inventories wait for the next startup's `/inventory`. A failed read, or an answer for another star, is an error.
  - Not seen as events yet: `stow` and `change_owner` (not yet tried with monitor running; `device.stowed` and `device.changed_owner` are registered).
  - `prospect.completed`: the payload has `origin` (the envelope's star), `stars` (just names) and `stars_generated` (its length); either mismatch is an error. `monitor` takes the placed replicant (in a heaven_vessel with a location, so not travelling) whose star is nearest the origin, looks up each new star with its `/replicants/{code}/stars/{star}` (`api.StarNear`), and stores each one (`PutStar`: position and entry point). A failed lookup, a missing origin or replicant star in `stars`, or no placed replicant, is an error. See `cmd/monitor/prospect.go`.
  - `blueprint.unlocked`: has no device, just the envelope's `replicant_code`. The payload has `device_type`, `short_description`, `description`, `resources`, `print_time`, `requires_autofactory`, `components` (null so far) and `source` (`achievement` so far), but not features, directives or capacities, so `monitor` reads `/blueprints` again and replaces the table (`store.ReplaceBlueprints`). It's an error if the listing doesn't include the unlocked type, or disagrees with the event about its `print_time` or `resources`. See `cmd/monitor/blueprint.go`.
  - An event for a device not in the store is an error, as is a handled event whose payload won't decode.
  - **Handlers fetch first, then write.** A handler (`cmd/monitor/apply.go`) reads the store and calls the API outside any transaction, and returns a `change`; `recordAndApply` then opens the transaction, records the event and makes the change. So no API call happens while a transaction is open. A handler that needs nothing but the event is wrapped with `direct`.
- **Archived events:** `~/replicant-archive/events-2026-11-08.db` holds real events from 2026-11-08 16:20:45 to 20:15:46 (2,373, IDs `1794115245975-0` to `1794129346070-0`), from before the store was first rebuilt from scratch. It has only the `events` table, in the same schema. Every event type seen so far is there, so use it to look up real payloads (`SELECT payload FROM events WHERE event = 'print.completed'`) and to build test fixtures, without asking the API. It includes 106 `travel.departed`, 106 `travel.arrived`, 16 `print.completed`, and 2 `directive.resume_failure`. Don't replay it into a live store: applying hours-old events on top of a fresh snapshot can leave stale state.
- **Opening the store read-only: use `PRAGMA query_only=1`, not `mode=ro`.** The database is in WAL mode, so every connection, readers included, needs `replicant.db-wal` and `replicant.db-shm` next to the file. SQLite deletes them when the last connection closes, and a `mode=ro` (or `sqlite3 -readonly`) connection can't create them. So a read-only open fails with "unable to open database file" whenever `monitor` isn't running. A normal connection with `PRAGMA query_only=1` can create them and still refuses every write. (`?immutable=1` also opens, but promises SQLite the file won't change, so it can read inconsistent data while `monitor` writes.) `store.OpenReadOnly` does this.
  - For poking around by hand: `sqlite3 -cmd "PRAGMA query_only=1" ~/.replicant/replicant.db`.
- Write helpers: `store.RecordEvent` (`INSERT … ON CONFLICT DO NOTHING RETURNING seq`, so it reports whether the event was new) and `store.LastEventID`, `store.PutSystem` (a `/locations/STAR` response: the star, its planets and its belts, replacing belts that are gone; an unknown resource type is an error), `store.ReplaceInventory` (replaces the whole table from `/inventory`; an unknown resource type, or a location or resource listed twice, is an error), `store.ReplaceBlueprints` (likewise from `/blueprints`; an empty listing, an unknown resource type or a device type listed twice is an error), `store.Attached` and `store.Detached`, `store.Transforming` (status and `completes_at` of a compacting or unfurling device), `store.ControlledStatus` (a digest's word on one device), `store.SetStatus`, `store.Adopted` and `store.Released`, `store.PutDevice` and `store.PutStar` (upserts; `PutStar` keeps a known `entry_point` and the scan counts), `store.Count`, `store.ReplicantInVessel` (the owner of a heaven_vessel carrying a `replicant_matrix`, assumed to be the replicant in it).

## Commands

| Command | Does |
|---|---|
| `auto_scan -n TRANSPORT...` | Plans survey work. Each transport must have `surge` and `attach`, be idle, and carry exactly one ami_survey_controller plus at least 3 survey_drones (all problems are reported together). Attached drones the controller doesn't control are adopted by it (`adopt`), then re-read to check; with `-n` they are only reported. It finds systems with a `relaying` ftl_relay and unscanned planets or moons, picks one seed system per team (max-min spread), grows a group from each seed (the group with the least unscanned work takes its nearest unclaimed system), gives each group the team nearest to it, and prints each team's route: nearest system first, then a short path. `-n` stops there. Otherwise all teams run at once. At each system the team goes to the entry point, detaches the controller and drones, sets `survey_system`, logs the controller's survey digests, waits for `directive.completed` (12h limit), recalls the crew to the entry point and reattaches it. A failed team stops and the others carry on. After a crash, recover a team by hand (reattach its crew); scanned systems drop out of the next plan. |
| `auto_salvage` | Detaches the miners, AMI controller and any maintenance_drones from transport `E62C8DC1` and puts the maintenance drones on `patrol`. Salvages every site at the star (not belts or `-OBJ-`, and never at SOL), then clears the maintenance drones' directive, brings everything back to the transport and re-attaches it, moves to the star with the most resource sites and repeats. Maintenance drones are remembered only for the current run, so after a crash, reattach them by hand. |
| `auto_ferry STAR FREIGHTER DEST` | One freighter: fill from STAR's planets and moons, deliver to DEST, repeat until the star is empty. |
| `slurp [-replicant ID]` | All idle (and, once arrived, travelling) cargo_freighters greedily move resources from planets and moons to the nearest belt already holding more than 1000. A ledger of claims (inventory re-read on each decision, minus outstanding claims) stops two freighters going for the same stockpile. Exits when nothing is left to claim. |
| `collect_ftl_relays VESSEL` | Stows the `inactive` ftl_relays at a heaven_vessel's exact location into it (`auto.StowInto`, which changes their owner to the vessel's replicant first). Exits on the first failure. |
| `plant_relays [-n] [-db PATH]` | Plants ftl_relays at every star in the store that can join the relay mesh, using every idle heaven_vessel carrying a `replicant_matrix` (`auto.ReadyReplicant`). It plans entirely from the store, so `-n` makes no API calls; a star with no `entry_point` counts as unscanned. Before a real run it checks the vessels, the relays to fetch and the mesh against the API, and stops if any differ. `monitor` applies its deploys and activations, but not stowing, ownership changes or scans (see `cmd/plant_relays/README.md`). The mesh is the stars with a `relaying` relay; a star can join if it's under 7.5 ly from a mesh star or another joining star (`connectable`, in growth order, so a prefix is still connected). Relays may be planted temporarily out of range, as long as the finished mesh is connected. **Phase 1 (`fetch.go`):** spare relays are the unstowed, non-relaying ones at `-BELT-` locations. Each vessel either stays or goes to one of its 4 nearest belts, and takes every relay there it has room for (`stow_capacity - stow_used`; vessels sharing a belt split it). All combinations are tried, preferring, in order: the fewest relays short, the fewest vessels left with none, the shortest longest trip to a belt, the shortest total. If still short, only the first N connectable stars are planned. **Phase 2 (`divide.go`):** one seed star per vessel holding relays (`auto.Seeds`), each seed given a vessel (`auto.Assign`, from where the vessel will be after fetching), then the vessel with the cheapest trip so far (fetch distance plus `auto.Order` route) that still has a spare relay takes the unclaimed star nearest its stars. `-n` prints the plan. Otherwise all vessels run at once: go to the belt, `auto.StowInto` each relay, then at each star go to the entry point (or `STAR-OORT` and `system_scan` first if it isn't known), deploy and check it's at the entry point, activate and check it's `relaying`. A vessel stops at its first failure; the others carry on. Spare relays stay on board. |
| `gaze [-n] [-db PATH]` | Cycles the galactic_observatories, planning from the store (stars, relay stars, where devices are) and checking the observatories' and chosen transports' statuses with the API (with `-n` too; any disagreement with the store stops it). Compact, attach to a surge transport (one at the same location, else in the same system, else the nearest idle one carrying nothing, which first comes over), travel to the next relay star's entry point (the star's `entry_point`, not wherever its `relaying` ftl_relay sits) (nearest the centre of relay stars first, skipping those listed in `$HOME/observed_from.txt`), detach, unfurl, and prospect pointing outward from the centre. It does the short steps, starts the next long one (compact, unfurl, prospect) and exits, so run it again later. `-n` prints the plan only. Spec in `cmd/gaze/README.md`. |
| `complete_events [-n] [-db PATH]` | Plans and does everything for location events (`/accounts/events`). Star positions, relay stars and devices (where they are, status, attachments, what's stowed in vessels, carriers' `attach_capacity` from blueprints) come from the store; events and stock (`/inventory`) come from the API. Before a real run every device the plans use is fetched and checked against the store (type, status, location, attachment, stowing, attach counts, a vessel's stowed devices), and any difference stops it; `-n` skips that check. **Rule: we never complete an event at a location without an `ftl_beacon`.** For each event it picks an option: a satisfied one (everything there), else the first whose missing devices we have spare (counting beacons first, and taking each choice off the spare count), else the first (FIXME: resources don't affect the choice). **Resources:** it works out what's missing (`required - current`) and looks for the nearest relay star whose planets, moons and belts (not event locations) hold all of it, using stock from `api.InventoryAt(ctx, "")`. Events are taken greedily, nearest source first, and each claim comes off the stock. Each sourced event gets at most one `idle` cargo_freighter, covering as many events as possible while keeping the longest freighter-to-source distance short. A freighter collects what fits (first unloading cargo that isn't for this delivery, if it needs the room), deposits just that at the event (`DepositResources`), and repeats until done. A stockpile that has shrunk is delivered short. **Devices:** each event without a beacon needs one, plus its option's missing devices. Spares are ftl_beacons at a `-BELT-` location, or other `idle` devices; neither may be attached, stowed, at an event location or at `-OBJ-`. Idle surge_carrier/platform/plate devices with free `attach_capacity` carry them, each device counting as 1. The planner (`devices.go`) is a pickup-and-delivery heuristic: each job considers its 4 nearest spares, and jobs are inserted greedily (furthest first) and then relocated. It aims for the earliest finish, then the least total travel. Cost is star distance plus 0.1 per move (FIXME: calibrate). At each stop a carrier detaches, then attaches only devices that are still free there, and checks `attached_devices` after each step. **Completions:** ready events (option satisfied, beacon present) are split among idle heaven_vessels carrying a `replicant_matrix`. It takes N = min(vessels, events) seeds with `auto.Seeds`, every other event joins its nearest seed, each group gets a vessel via `auto.Assign`, and each route follows `auto.Order`. A vessel travels to each event and POSTs the completion; a failed completion is logged and the vessel moves on. `-n` prints the three plans. Otherwise freighters, carriers and vessels all run at once, and a failed device stops while the others carry on. Events made ready by this run are completed by the next. |
| `start_belt [-n] [-db PATH]` | Starts mining an undeveloped belt, planning entirely from the store (which it assumes is accurate, statuses and attachments included). Candidates are belts at relay stars with no ami_mining_controller there or travelling there. They're ranked by density (dense > moderate > sparse), then, from `rich` down to `low`, how many of volatiles and rares are at that level, then how many other resources are; ties go to the nearest team. An unknown density or level is an error. A team is one location with a free (idle, unattached, unstowed) ami_mining_controller, ami_survey_controller, maintenance_drone, belt_surveyor and 5 mining_drones, the last two with no controller; the team nearest the belt (star distance) is used. The carrier is the nearest free surge_carrier (capacities are hardcoded: surge_plate 1, surge_platform 4, surge_carrier 9, so the team is 9). `-n` prints the ranked belts with their levels, the team and the carrier. Otherwise the carrier goes to the team, attaches it (checking `attached_devices`), goes to the belt and detaches it; the mining controller adopts the miners (each re-read to check) and gets `gather_evenly`, the survey controller adopts the surveyor and gets `belt_search`, and the maintenance drone patrols. It stops at the first failure, leaving recovery to you; the carrier stays at the belt. |
| `print_queue [-db PATH]` | Reports what each autofactory is printing and has queued, by location. It takes the autofactories from the store, reads each one's `printing` and `print_queue` with `/devices/{code}`, and prints each job with its duration and ETA: the current print's from its `completes_at`, then each queued item's blueprint `print_time` (from the store), chained after the previous job. A factory that isn't printing (e.g. `waiting_for_resources`, shown with its `waiting_for` as `C 10/50`) gets durations but no ETAs. A queued type with no blueprint is an error. |
| `migrate_db [-db PATH] [-allow-drops]` | Brings the database into line with `store/schema.sql`. Every command that opens the store does this anyway; this is for `-allow-drops`. |
| `monitor [-n] [-db PATH] [-stars 60] [-since ID] [-catchup EVENT,...]` | Keeps the store up to date, and is the only program that writes to it. It first connects to the event stream, resuming after the last event in `events` (or after `-since ID`, to replay), and queues events while it does its startup work. Startup: if `devices` is empty it fetches every device. If `stars` is empty it fetches the stars nearest the replicant in a heaven_vessel, then reads `/locations/STAR` for each (`store.PutSystem`), skipping systems we haven't scanned; any other error stops it. That is one transaction, so a failure leaves `stars` empty and the next run starts again. Every run, it replaces the `inventory` table from `/inventory` and the `blueprints` table from `/blueprints`. With `-catchup EVENT,...` (e.g. `prospect.completed`, `blueprint.unlocked`, or `device.deployed,relay.activated`) it then applies every recorded event of those types again, oldest first across all of them (so a relay's deploy comes before its activation), each in its own transaction, as if it had just arrived (but without recording it again): use it when monitor learns to handle an event type that's already in the log. An event type it doesn't handle is an error. Next, in one transaction (`cmd/monitor/survey.go`), it files a pending `survey_system` task (`location` = the bare star) for each relay star that isn't fully surveyed and has no task yet (not even a failed one), then assigns pending, unassigned survey tasks to free survey teams, at most one each, minimising the longest trip and then the total (`auto.MatchMinMax`, star-to-star distance). A free team is an ami_survey_controller with a location, at least 5 survey_drones (assumed to be with it), no belt_surveyor, and no task. Its status doesn't matter, and nothing carries the tasks out yet; other commands ignore tasks. Then, in another transaction (`cmd/monitor/move.go`), it gives pending `move_devices` tasks to free transports, at most one each, with the same matching: as many as possible, then the shortest longest trip from the transport's star to the task's, then the total. A transport has `surge` and `attach` in its features; it must have room for every device in the move (`blueprints.attach_capacity`, each device counting 1), and be free: it has a location, isn't attached or stowed, carries nothing at all, isn't any task's assignee or claimed, and isn't one of the devices a move is to move (`store.FreeTransports`). A move nothing can carry stays pending. `-n` prints both plans from the store as it is (read-only, no API calls) and exits; it still needs `$API_KEY` set, to any value. Then it calls the task handler for every `assigned` task, highest priority (lowest number) first, then oldest (`cmd/monitor/tasks.go`). Each task type has one handler, named after it in camelCase (`survey_system` -> `surveySystem`) and listed in `taskHandlers`; it gets the `store.Task` (id, location, priority, assignee, details). Like an event handler, a task handler reads the store and calls the API (commands included) but doesn't write: it returns a `change`, applied in its own transaction. Each handler takes one step and returns; nothing calls it again until the next startup (event dispatch is still to come). A task type with no handler, or a failing handler, stops it. **This sends real commands at startup.** `surveySystem` (`cmd/monitor/survey.go`) fetches the controller and every device it controls from `/devices/{code}`, then `decideSurvey` picks one step: anyone with no location (travelling or carried): wait. In the target star: directive `survey_system` active: wait; survey incomplete: send `survey_system`; anyone away from the entry point: `travel` them there; otherwise finish the task (`store.FinishTask`, archived as done) and give the controller its next task (`assignNext`). Elsewhere: anyone away from the current star's entry point: `travel` them there; otherwise file a `move_devices` task (`location` = that entry point, `details` = `{"dst": target entry point, "devices": [...]}`) and block on it. Errors: a team split across stars, a member attached to a carrier when it must act (`move_devices` is to leave the team detached at the target), a controller directive other than `survey_system`, or any directive outside the target. `assignNext` is the generic "what should this device do next": of the pending tasks its type can do (`taskTypesFor`: survey controllers take `survey_system`, transports `move_devices`) and that fit it (`taskFits`: a move must fit the transport's capacity), the highest priority, then nearest. `moveDevices` (`cmd/monitor/move.go`) fetches its transport from `/devices/{code}` (and the passengers, only when at the pickup, or at the destination with nothing attached), then `decideMove` picks one step: travelling: wait; at the destination with every passenger attached: `detach` them, re-fetch the transport to check it carries nothing, finish the task (unblocking what waited for it) and `assignNext`; at the destination, nothing attached, passengers there: just finish; every passenger aboard: travel to the destination; at the pickup: `attach` the passengers not yet attached, re-fetch to check it carries exactly the passengers, travel to the destination; nothing attached: travel to the pickup. Errors: carrying a device that isn't a passenger, a passenger not detached and waiting at the pickup (a move assumes they're all there), not enough room, some passengers attached away from the pickup, or a destination that isn't a relay star (`store.CheckRelayStar`). Tasks a handler unblocks or assigns don't run until the next startup, on purpose for now. Survey completion reads `stars.is_fully_scanned`, which the survey's digests and its `directive.completed` keep current. Then, until interrupted (Ctrl-C), it records each event in `events` (`store.RecordEvent`, even ones that wouldn't decode), applies it to the store if it's one we handle (in the same transaction, so a failure leaves it unrecorded and the next run retries it), and prints it. Applied so far: travel (`departed`, `arrived`), `print.completed`, `device.attached`/`detached`, AMI digests (mining, survey), `ami.adopted`/`released`, `directive.set`/`completed`/`cleared`, `device.compacting`/`compacted`/`unfurling`/`unfurled`, `device.deployed`, `relay.activated`, `prospect.completed`, `blueprint.unlocked` and scan `experience.gained` (see `cmd/monitor/apply.go`, `modular.go`, `stow.go`, `scan.go`, `prospect.go` and `blueprint.go`). Survey and mining digests and `site.depleted` share a line; an undecodable event is shown as `UNDECODED` with its raw JSON. If the stream drops it reconnects with backoff. An event that's already recorded stops it, unless replaying with `-since`. Any failure to apply an event stops it. It replaced `sse_demo`. |
| `star_map` | Writes and opens an interactive 3D Plotly map of the nearest stars, centred on the mean of the explored ones. |
| `summarize_devices [DEVICE_TYPE]` | Devices by type (or just DEVICE_TYPE, e.g. `heaven_vessel`), then by star, with status counts: `"OTHYONAL" monitoring(3)`. |
| `stars`, `observatory_plan`, `sqlite_demo` | Older experiments and demos. |

Don't run `slurp` and `auto_ferry` on the same freighter at once.

**auto_scan grouping, future work:** grouping currently balances only the scanning work (unscanned planets and moons). It should instead add each system to the group where it adds the least total work, counting both scanning time and travel time. Travel time is roughly linear in distance, so it's easy to model. We don't have a model for how long a scan takes yet (per planet, per moon, and how it varies with the number of drones). Once we do, both costs can be put in the same unit (time) and compared.

### How we build a new command

1. Talk it through first: list the open questions (edge cases, starting state, when to stop, what to do on failure) and agree the answers before writing code.
2. Look up the endpoint or command in the docs. Where the docs are silent or wrong, ask for, or capture, a real response.
3. Add any missing endpoint wrapper to `api` (thin), any reusable logic to `auto`, and the rest to `cmd/<name>/`.
4. Use the hub for anything asynchronous. Register first, then act, then wait.
5. Log what's happening, especially before and after each action, so a run can be followed from the terminal.
6. Unit-test the decision logic, then build, vet and test. Say clearly what hasn't been tried against the live API.

Long-running tools stop when the Mac sleeps: run them with `caffeinate -i go run ./cmd/...`.

## UI

An Angular 22 app in `ui/` (standalone components, signals, `@if`/`@for` templates). Start it with `npm start` (`ng serve`). `proxy.conf.json` forwards `/v1` to the API.

- **Layout:** sections are `<details>` blocks in `src/app/app.html`, laid out in three columns. Each section is a component in `src/app/<name>/` with `.ts`, `.html`, `.scss` and `.spec.ts` files.
- **API calls:**
  - Calls go through the `ReplicantApi` service (`src/app/replicant-api.ts`); types live in `src/app/model.ts`.
  - Newer components call an async method on it (e.g. `api.inventory()`, `api.reputation()`) from a **Fetch** button, and put errors in `api.errorMessage`.
  - `pagedGet` handles cursor paging.
- **Location Events:** its **Fetch** lists every active event from `/v1/accounts/events?status=active` (`api.accountEvents()`, paged), sorted by location, with tier, request and reward. A request shows one line per alternative, like `C(0/100), defence_grid(0/1)` (on-site/required). An alternative gets a ✅ when everything in it is there (`optionSatisfied`), whether or not a replicant is present; the API's own `met` waits for a replicant. Resources are abbreviated C, Co, R, Si, S, V. The reward shows resources only, like `C500, R50`.
- **Auth:** the API key typed into Account is saved in `localStorage`, and `authInterceptor` adds it to `/v1` requests.
- **Testing:** use `npx ng test --watch=false --include src/app/<name>`. The full run currently fails to compile because of `device.spec.ts`.
- **Dev-server quirk:** `ng serve` sometimes misses newly added files. Touching `src/app/app.ts` makes it rebuild.
