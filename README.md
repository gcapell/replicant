# replicant

Go automation tools and an Angular dashboard for the [replicant.space](http://replicant.space/) game API.

- API docs: <http://replicant.space/docs/> (each endpoint has its own page, e.g. `/docs/api/devices/command/`)
- Base URL: `https://api.replicant.space/v1`, with `Authorization: Bearer $API_KEY`

This file records how the code is put together and the conventions we've settled on, so that a new piece of work can start from here.

## Layout

| Path | What |
|---|---|
| `api/` | Thin HTTP client: one small function per endpoint or command, plus the SSE event hub. |
| `model/` | Response structs (`model.go`, `device.go`) and SSE event payloads (`sse.go`). |
| `auto/` | Shared higher-level helpers used by the commands (device listing, travel-and-wait, location tests). |
| `cmd/<name>/` | One `main` package per tool. |
| `ui/` | Angular dashboard (see [UI](#ui)). |

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
  - travel and cargo: `Travel`, `CollectResources`, `DepositAllResources`;
  - attaching and AMI: `AttachDevice`, `DetachDevice`, `AdoptDevice`, `Salvage`;
  - ownership and stowing: `ChangeOwner`, `Stow`, `Deploy`;
  - relays and scanning: `Activate`, `SystemScan` (returns the system's `entry_point`);
  - modular devices and observatories: `Compact`, `Unfurl`, `Prospect` (takes a `direction`).
- **Errors:** a non-2xx response is an `*api.HTTPError`; use `errors.As` to read its `StatusCode` and `Body`.
- **Galactic observatory** status values seen so far: `compacted`. Its features include `cruise` (in-system only) and `modular`. A compacted observatory needs a transport with `surge` and `attach` to move between systems.
- **Every request is logged** (`GET /path`, or `POST /path {body}`).

### API facts we've confirmed

- **Device status** values include `idle`, `travelling`, and `inactive` (an FTL relay that isn't relaying).
- **Moving devices:**
  - `stow` is sent to the *device being stowed*: `{"command":"stow","target":<host device>}`. It is immediate: no error means it worked.
  - `change_owner` is sent to the device: `{"command":"change_owner","target":<replicant_code>}`. Both replicants must be on the same account.
  - `deploy` is sent to the *stowed device*, with no arguments (as the UI's deploy button does). It's immediate: fetch the device straight afterwards and its `location` is the host's.
  - `activate` is sent to the deployed ftl_relay and takes effect at once: its status should be `relaying` straight afterwards.
  - `attach` and `detach` are sent to the carrier with `targets: [...]`. The two devices must be at the same location.
- **Travel and cargo:**
  - `travel` takes just a `destination`. Surge-capable devices (e.g. cargo_freighter) work out their own route.
  - `collect_resources` and `deposit_resources` are synchronous; we still look for the confirming event (see the hub section).
- **Inventory:** `/inventory` `items` is a **list** of `{resource_type, quantity}`, although the docs show a map. Handle both in the UI.
- **Locations:**
  - `/v1/locations` returns a map of location code to `{devices, location_events, replicants, resource_sites, resources}`. Moons' salvage sites only appear here, not under `/locations/PLANET`.
  - `/v1/devices?location=X` takes either a star (`MIRETH`) or an exact location (`MIRETH-4-L4`).
  - `/v1/locations/STAR` (`api.Location`) gives the system: `system_scanned`, `planets_scanned`/`planets_total`, `moons_scanned`/`moons_total` (`Location.Unscanned()` sums what's left), `entry_point`, `planets`, and `star.position`.
- **Relays:** only send devices to a system whose ftl_relay is `relaying`. Anything sent to a system without an active relay can't be controlled until a replicant goes there to rescue it.
- **System scan:** `{"command":"system_scan"}` sent to a vessel scans the system it's in. This is what `/replicants/{code}/scan` runs. The response's `entry_point` (e.g. `CORBRENEUS-5-L4`) is the system's easiest-to-reach L4 point, which is where we put relays. `/stars` also returns `entry_point` for stars we already know it for.
- **Stars:**
  - `/v1/replicants/{code}/stars?per_page=30` gives the nearest stars, with positions (`api.Stars(ctx, code, perPage)`). Our replicant is `4698E8E6`.
  - **Never fetch `/v1/stars`.** That's the whole catalogue, around a million stars, and it's rate limited.
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
  - travel: `travel.departed` (has `TravelTimeSeconds`, used for the ETA), `travel.arrived` (sent only at the end of a trip, not for intermediate legs, so any arrival for the device ends the wait. A surge to another system reports the bare star as `destination`, e.g. `"PARBEA"`, with the real place in the event's `Location`, e.g. `PARBEA-OORT`), `travel.cancelled`;
  - cargo: `transport.collected`, `transport.delivered`.
- **Synchronous commands** (collect, deposit) treat a missing confirmation as a warning, after a 10s timeout. Long waits (travel) use 12h.

### Travel helpers (`auto/travel.go`)

- `auto.GoTo(ctx, hub, device, loc)`: send and wait for arrival. If the device is already there it does nothing; if it's already heading there it just waits. It logs `X travelling A -> B (ETA 3m22s)`.
- `auto.GoToAll`: run `GoTo` for several devices concurrently.
- `auto.FinishTrip`: wait out whatever trip a device is on.
- `auto.ShortRoute(start, points, pos)`: a short open path through points (nearest neighbour, then 2-opt), with `auto.RouteLength`.
- Device listing helpers: `auto.Devices(ctx, type)`, `auto.Everything(ctx, loc)` (all types), `auto.DevicesInSystem(ctx, star, type)` and `auto.DevicesAt(ctx, loc, type)`.

## Commands

| Command | Does |
|---|---|
| `auto_scan -n TRANSPORT...` | Plans survey work. Each transport must have `surge` and `attach`, be idle, and carry exactly one ami_survey_controller plus at least 3 survey_drones that it controls (all problems are reported together). It finds systems with a `relaying` ftl_relay and unscanned planets or moons, picks one seed system per team (max-min spread), grows a group from each seed (the group with the least unscanned work takes its nearest unclaimed system), gives each group the team nearest to it, and prints each team's route: nearest system first, then a short path. Only `-n` (dry run) is implemented. |
| `auto_salvage` | Detaches the miners and AMI controller from transport `E62C8DC1`, salvages every site at the star (not belts or `-OBJ-`), re-attaches them, then moves to the star with the most resource sites and repeats. |
| `auto_ferry STAR FREIGHTER DEST` | One freighter: fill from STAR's planets and moons, deliver to DEST, repeat until the star is empty. |
| `slurp [-replicant ID]` | All idle (and, once arrived, travelling) cargo_freighters greedily move resources from planets and moons to the nearest belt already holding more than 1000. A ledger of claims (inventory re-read on each decision, minus outstanding claims) stops two freighters going for the same stockpile. Exits when nothing is left to claim. |
| `collect_ftl_relays VESSEL` | Stows the `inactive` ftl_relays at a heaven_vessel's exact location into it, changing their owner to the vessel's replicant first. Exits on the first failure. |
| `plant_relays [-n] [-stars 50] VESSEL` | Checks the heaven_vessel is idle and carrying ftl_relays, then plans a route through the nearest stars without a relay. Each stop must be under 7.5 ly from a relay star: an existing one (active or inactive) or one planted earlier on the route. It picks the nearest reachable star each time, then shortens the order with 2-opt moves that keep that rule. The first N stops (N = relays on board) are planned on their own; the rest continue from there, and stars out of reach are listed with their nearest relay star. It then follows the plan for as many stops as it has relays. At each stop it goes to the entry point (or, if that isn't known yet, to `STAR-OORT` and runs a `system_scan` first), deploys a relay and checks it's at the entry point, then activates it and checks it's `relaying`. It exits at the first failure. `-n` prints the plan only. |
| `gaze [-n] [-stars 50]` | Cycles the galactic_observatories: compact, attach to a surge transport, travel to the next relay star's entry point (nearest the centre of relay stars first, skipping those listed in `$HOME/observed_from.txt`), detach, unfurl, and prospect pointing outward from the centre. It does the short steps, starts the next long one (compact, unfurl, prospect) and exits, so run it again later. `-n` prints the plan only. Spec in `cmd/gaze/README.md`. |
| `star_map` | Writes and opens an interactive 3D Plotly map of the nearest stars, centred on the mean of the explored ones. |
| `summarize_devices` | Devices by type, then by star, with status counts: `"OTHYONAL" monitoring(3)`. |
| `stars`, `observatory_plan`, `sse_demo`, `sqlite_demo` | Older experiments and demos. |

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
- **Auth:** the API key typed into Account is saved in `localStorage`, and `authInterceptor` adds it to `/v1` requests.
- **Testing:** use `npx ng test --watch=false --include src/app/<name>`. The full run currently fails to compile because of `device.spec.ts`.
- **Dev-server quirk:** `ng serve` sometimes misses newly added files. Touching `src/app/app.ts` makes it rebuild.
