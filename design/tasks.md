# Task model

It might help to organise our work if we maintain a database of _tasks_, e.g. "mine all salvage locations at OTHYONAL".

## Decisions so far

- **`monitor` becomes the executor** (and probably gets renamed, e.g. `executor`). It stays the only program that writes to the store. It already has the event stream, the store and the "fetch first, then write" pattern.
- **Commands get phased out.** In the end each one is a thin wrapper that adds a task. Until then, a command must refuse devices that are assigned to an active task (see [Free devices](#free-devices)).
- **Handlers are reconcilers.** A task handler (`addRelay(task)`, `surveySystem(task)`, ...) looks at the world (the store, plus the API where things must be exactly current), takes the *next* step, and returns. It doesn't sit in a goroutine waiting for travel to finish. An event wakes it, and it runs again. So every handler has to be safe to run again at any point. That's what makes restarts work: on startup, run the handler once for every assigned task. Existing helpers such as `auto.GoTo` (which blocks until arrival) will need rewriting as steps ("am I there? if not, am I on my way? if not, send `travel`").
  - Handlers run in goroutines, so a slow API call doesn't hold up the event loop. Their store writes go back through the executor's single writer.
- **No central scheduler, and no batch assignment.** Whatever finishes a step picks the next piece of work: the event-handling thread, or a handler finishing its step. Greedy choice: priority first, then distance. This is less efficient than the global planners in `plant_relays` and `complete_events`, but it's simpler, and that's the trade-off we want for now.
  - The claim must happen *inside* the write transaction that records it: "find a free device" and "assign it" happen in one transaction. That way two handlers finishing at the same moment can't both take the same device. (`_txlock=immediate` serialises them, and the unique index on `assignee` backs it up.)
- **One assignee per task**, by convention:
  - an `ami_*_controller` stands for itself plus every device it has adopted (`devices.controller`);
  - a mining controller has one maintenance_drone of its own (`adopted_maintenance_drones`), which goes wherever the controller goes. Pairing a particular drone with a controller, rather than taking "any one at my location", stops two teams at the same place from both counting on the same drone. This is team membership, not a task claim, so it's kept from task to task;
  - a transport on a `move_devices` task carries the devices listed in its `details`.
- **Finished tasks move to `archived_tasks`**, in the transaction that finishes them, so `tasks` stays small.

## Schema

```sql
CREATE TABLE tasks (
	id INTEGER PRIMARY KEY,
	task TEXT NOT NULL,      -- e.g. 'salvage'
	location TEXT,           -- e.g. 'OTHYONAL' (salvage), 'OTHYONAL-1' (event delivery)
	star TEXT AS (CASE WHEN instr(location, '-') = 0 THEN location
	                   ELSE substr(location, 1, instr(location, '-') - 1) END) STORED,
	priority INTEGER NOT NULL DEFAULT 3 CHECK (priority BETWEEN 0 AND 5), -- 0 is highest
	status TEXT NOT NULL DEFAULT 'pending'
		CHECK (status IN ('pending', 'blocked', 'assigned', 'failed')),
	assignee TEXT,           -- device code; NULL until assigned
	details TEXT,            -- task-specific JSON, e.g. {"carbon":400,"silicates":100}
	error TEXT,              -- why it failed
	created_at INTEGER NOT NULL,  -- Unix ms
	updated_at INTEGER NOT NULL   -- Unix ms
) STRICT;

-- A device works on at most one task at a time.
CREATE UNIQUE INDEX tasks_by_assignee ON tasks(assignee) WHERE assignee IS NOT NULL;

-- Creating tasks is idempotent: startup scans and events can both propose
-- 'add_relay at ASTRANOVA' without making two. A second deliver_resources to
-- the same location is merged into the existing task's details instead.
CREATE UNIQUE INDEX tasks_unique ON tasks(task, location) WHERE status != 'failed';

-- task can't proceed until dep is finished.
CREATE TABLE blockers (
	task INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
	dep INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
	PRIMARY KEY (task, dep)
) STRICT;
CREATE INDEX blockers_by_dep ON blockers(dep);

-- Resources a task has earmarked, so no other task counts on them.
CREATE TABLE claimed_resources (
	task INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
	location TEXT NOT NULL,
	resource TEXT NOT NULL,  -- e.g. 'carbon'
	quantity INTEGER NOT NULL CHECK (quantity > 0),
	PRIMARY KEY (task, location, resource)
) STRICT;

-- Loose devices a task has earmarked for itself, e.g. the spare ftl_beacons
-- a deliver_devices task will pick up. A device can be claimed by only one task.
CREATE TABLE claimed_devices (
	device TEXT PRIMARY KEY,
	task INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE
) STRICT;
CREATE INDEX claimed_devices_by_task ON claimed_devices(task);

-- Team membership: each mining controller's own maintenance_drone.
-- One drone per controller, and one controller per drone. Not tied to any
-- task, so it lasts from task to task. No foreign key to devices, which
-- monitor upserts from snapshots.
CREATE TABLE adopted_maintenance_drones (
	drone TEXT PRIMARY KEY,
	controller TEXT NOT NULL UNIQUE
) STRICT;

-- Finished (done or failed) tasks, moved out of tasks.
CREATE TABLE archived_tasks (
	id INTEGER PRIMARY KEY,  -- the id it had in tasks
	task TEXT NOT NULL,
	location TEXT,
	priority INTEGER NOT NULL,
	status TEXT NOT NULL CHECK (status IN ('done', 'failed')),
	assignee TEXT,
	details TEXT,
	error TEXT,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	finished_at INTEGER NOT NULL
) STRICT;
```

Notes:

- SQLite enforces `REFERENCES` only with `PRAGMA foreign_keys=ON`, so `store.Open` needs to set it.
- Archiving a task means: insert it into `archived_tasks`, then delete it from `tasks`. The cascade removes its blockers and claims (but not `adopted_maintenance_drones`, which isn't about tasks).
  - Deleting a `blockers` row can leave its `task` unblocked. The same transaction should look for tasks with `status = 'blocked'` and no remaining `blockers` rows, set them to `assigned` (or `pending` if they have no assignee), and wake their handlers once it commits.
  - **A failed dep** must not silently unblock its parent. Archiving a dep as `failed` should wake the parent, which decides what to do: retry, file a new dep, or fail too.
- `failed` stays a status in `tasks` so that a human can look at a failure before it's archived (and so that `tasks_unique` lets the task be filed again).

### Free devices

**Belt search teams stay out of tasks.** Belts are surveyed only by belt_surveyors, and a belt_surveyor plus the ami_survey_controller that adopted it never move and never take part in tasks. So an ami_survey_controller that has adopted any belt_surveyor is not assignable, and a system survey team (an ami_survey_controller with survey_drones) never includes belt_surveyors.

Only _assignable_ devices take part in tasks: ami_*_controllers (except belt search controllers, above), transports (surge carriers, cargo_freighters) and heaven_vessels carrying a replicant. Devices a controller adopts come along with it, and are never assigned in their own right. So a free device is just an assignable one that isn't any task's `assignee`. A `free_devices` view lets the handlers and the remaining commands all ask the same question.

A team's maintenance_drone belongs to its mining controller through `adopted_maintenance_drones`. Other loose devices a task needs (e.g. the spare ftl_beacons `deliver_devices` picks up) are reserved in `claimed_devices`. A loose device is free if it isn't adopted by a controller, isn't in `adopted_maintenance_drones`, and isn't claimed.

## Task types

Finite tasks (archived once done):

- `add_relay`: add a relay at a system.
- `survey_system`: scan the planets and moons at a system.
- `salvage`: collect all the salvage from a system.
- `move_devices`: move a group of devices from a source to a destination.
- `complete_event`: complete the event at a location.
- `deliver_resources`: bring exactly the given resources (from anywhere) to a location, e.g. 300 carbon to X. There's at most one per location; a new request is added to its `details` (`{carbon:100}` + `{carbon:100, silicates:100}` -> `{carbon:200, silicates:100}`). Because the target can grow mid-flight, `details` should record what has been delivered so far as well as the target, and the handler delivers the difference.
- `deliver_devices`: bring the given devices (from anywhere) to a location.

Continual tasks (never finish; they hold their assignee until cancelled):

- `mine`: at a belt.
- `consolidate`: keep moving all resources in a system to one location, e.g. OTHYONAL-3-L4.
- `ferry`: keep moving resources between two interstellar locations, e.g. ANTHALON-1-L4 -> OTHYONAL-3-L4.

## Event dispatch

When the executor applies an event, it also works out which tasks the event concerns, and wakes their handlers after the transaction commits. For example:

- `travel.arrived` / `travel.departed` / `travel.cancelled`: the task whose assignee is the device.
- `directive.completed` / `directive.cleared` / `ami.*.digest`: the task whose assignee is the controller.
- `relay.activated`: the `add_relay` task at that star.
- `prospect.completed`: no task. It *creates* tasks (see below).

## Extended example

* We receive a `prospect.completed` event, telling us about 4 new stars.
  * We check and find that three of those could be added to our relay network (the fourth is too far out).
  * We look for assignees: free heaven_vessels carrying a replicant (ignoring pre-emption of lower-priority tasks for now). We find two.
  * In one transaction we:
    * record the `prospect.completed` event (and apply it to `stars`, as now);
    * add three `add_relay` tasks, two of them `assigned` and one `pending`.
  * After committing, we start `addRelay(task)` for each of the two assigned tasks.

* `addRelay(237)` (vessel V, ASTRANOVA) runs, sees V isn't at ASTRANOVA's entry point, and sends `travel`. On `travel.arrived` it runs again: it deploys and activates the relay.
* We receive `relay.activated` at ASTRANOVA. It wakes task 237, whose handler sees a `relaying` relay at the star, so it is done. In one transaction it:
  * archives the `add_relay` task as `done`;
  * looks for the closest free survey team (an ami_survey_controller with its adopted drones), and finds C at WIRUN;
  * adds a `survey_system` task for ASTRANOVA with assignee C;
  * gives V its next task (by priority, then distance), e.g. the third, still `pending`, `add_relay`.
* After committing, it starts `surveySystem(task)` for C, and the handler for V's new task.

* `surveySystem` sees that C's team is at WIRUN, not ASTRANOVA, and no transport is free. In one transaction it:
  * adds a `move_devices` task: `{location: WIRUN, details: {dst: ASTRANOVA, devices: ["abc", "def", "123", ...]}}`, `pending`;
  * adds a `blockers` row (survey task, move task) and sets the survey task to `blocked`.

* Transport T, which was carrying out some other `move_devices`, gets `travel.arrived`. Its handler detaches what it was carrying, then in one transaction:
  * archives its old `move_devices` task as `done` (unblocking whatever was waiting on it);
  * picks its next task (by priority, then distance): our WIRUN -> ASTRANOVA `move_devices`, and assigns itself.
* Once committed, T's handler runs again for the new task: it sends `travel` to WIRUN. On arrival it attaches the team and travels to ASTRANOVA.

* We receive `travel.arrived` for T at ASTRANOVA. T's handler detaches the team, then in one transaction:
  * archives the `move_devices` task as `done`, which removes the `blockers` row;
  * finds the `survey_system` task has no blockers left and sets it back to `assigned`;
  * picks T's next task.
* After committing, it wakes `surveySystem` for C. This time the team is in the right system, so it sends `survey_system`.

* We receive `directive.completed` for C. It wakes the `survey_system` task. The handler checks `stars.is_fully_scanned` for ASTRANOVA (see the prerequisite below), moves the team to the system's entry point (for faster collection; again one step per wake), and then in one transaction:
  * archives the `survey_system` task as `done`;
  * adds a `salvage` task if the survey found salvage sites (assigning a free team if there is one);
  * picks this team's next task (by priority, then distance).
* If it was given a new task, its handler starts.

**Prerequisite:** today `monitor` reads `/locations/STAR` only the first time a star's system is scanned (when `planet_count` is NULL). After that the scan counts don't change, so `is_fully_scanned` goes stale during a survey. `monitor` needs to update the counts, either from `ami.survey.digest`'s progress (scanned/total) or by reading the system again on `directive.completed`.

## Startup

In steady state, events create (and finish) tasks. When the executor starts, it also:

1. runs the handler once for every `assigned` task (they're reconcilers, so this resumes whatever was under way);
2. runs the task finders that events would otherwise trigger: systems that could join the relay network, systems with salvage, active location events, ... `tasks_unique` makes it safe to propose tasks that already exist;
3. assigns free devices to `pending` tasks, as above.

## Open questions

- Cancelling a continual task, and what its assignee does afterwards. Ignored for now, but wanted soon: e.g. to move a belt-mining team temporarily onto salvage, which pays better.- How a remaining command adds a task when the executor is the only writer. Perhaps a small request API on the executor, or a `task_requests` table as the one exception to the single-writer rule.
- How a new belt gets its belt search team, given that those teams never move. Today `start_belt` carries a belt_surveyor and an ami_survey_controller there with the miners. Perhaps that first delivery is allowed, and the team stays put after it.
- Pre-emption: a high-priority task taking a device from a lower-priority one.
