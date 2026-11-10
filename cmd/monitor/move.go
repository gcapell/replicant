package main

// move_devices tasks (design/tasks.md): a transport carries Devices from the
// task's location to Dst. At startup, assignMoves gives the pending ones to
// free transports. moveDevices is the handler.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/auto"
	"github.com/gcapell/replicant/model"
	"github.com/gcapell/replicant/store"
)

const moveDevicesTask = "move_devices"

// moveDetails is a move_devices task's details: bring Devices from the
// task's location to Dst.
type moveDetails struct {
	Dst     string   `json:"dst"`
	Devices []string `json:"devices"`
}

// parseMove reads a move_devices task's details. No destination or no
// devices is an error.
func parseMove(t store.Task) (moveDetails, error) {
	var m moveDetails
	if err := json.Unmarshal([]byte(t.Details), &m); err != nil {
		return m, fmt.Errorf("task %d: details %q: %w", t.ID, t.Details, err)
	}
	if m.Dst == "" || len(m.Devices) == 0 {
		return m, fmt.Errorf("task %d: details %q: want a dst and some devices", t.ID, t.Details)
	}
	return m, nil
}

// blockOnMove files a move_devices task taking devices from src to dst, with
// task's priority, and makes task wait for it.
func blockOnMove(task store.Task, src, dst string, devices []string) taskChange {
	return func(ctx context.Context, tx *sql.Tx) ([]int64, error) {
		details, err := json.Marshal(moveDetails{Dst: dst, Devices: devices})
		if err != nil {
			return nil, err
		}
		now := time.Now()
		dep, err := store.AddTask(ctx, tx, moveDevicesTask, src, string(details), task.Priority, now)
		if err != nil {
			return nil, err
		}
		if err := store.Block(ctx, tx, task.ID, dep, now); err != nil {
			return nil, err
		}
		log.Printf("task %d: blocked on new %s task %d %s -> %s", task.ID, moveDevicesTask, dep, src, dst)
		// Give it (and any other waiting move) a free transport now.
		return assignPendingMoves(ctx, tx, log.Writer())
	}
}

// moveFits reports whether a transport of capacity can carry move m.
func moveFits(m moveDetails, capacity int) bool { return len(m.Devices) <= capacity }

type moveAssignment struct {
	Task      store.Task
	Move      moveDetails
	Transport store.Transport
	Distance  float64 // ly, from the transport's star to the task's
}

type movePlan struct {
	Assign  []moveAssignment
	Waiting []store.Task // pending tasks no free transport can take
}

// planMoves pairs free transports with pending move_devices tasks, each
// transport carrying at most one move, and only moves it has the capacity
// for. It makes as many pairs as it can, then minimises the longest trip
// from a transport to its task's location, then the total (star to star).
func planMoves(tasks []store.Task, transports []store.Transport, pos map[string]model.Position) (movePlan, error) {
	var p movePlan
	moves := make([]moveDetails, len(tasks))
	for i, t := range tasks {
		m, err := parseMove(t)
		if err != nil {
			return p, err
		}
		moves[i] = m
	}
	starPos := func(star string) (model.Position, error) {
		q, ok := pos[star]
		if !ok {
			return q, fmt.Errorf("no position for star %s", star)
		}
		return q, nil
	}
	dist := make([][]float64, len(transports))
	for a, tr := range transports {
		from, err := starPos(tr.Star)
		if err != nil {
			return p, err
		}
		dist[a] = make([]float64, len(tasks))
		for i, t := range tasks {
			to, err := starPos(auto.StarOf(t.Location))
			if err != nil {
				return p, err
			}
			dist[a][i] = math.Inf(1)
			if moveFits(moves[i], tr.Capacity) {
				dist[a][i] = from.Distance(to)
			}
		}
	}
	taken := make([]bool, len(tasks))
	for a, i := range auto.MatchMinMax(len(transports), len(tasks), func(a, i int) float64 { return dist[a][i] }) {
		if i >= 0 {
			p.Assign = append(p.Assign, moveAssignment{Task: tasks[i], Move: moves[i], Transport: transports[a], Distance: dist[a][i]})
			taken[i] = true
		}
	}
	slices.SortFunc(p.Assign, func(a, b moveAssignment) int { return int(a.Task.ID - b.Task.ID) })
	for i, t := range tasks {
		if !taken[i] {
			p.Waiting = append(p.Waiting, t)
		}
	}
	return p, nil
}

// readMovePlan gathers what planMoves needs from the store and plans.
func readMovePlan(ctx context.Context, q store.Reader) (movePlan, error) {
	tasks, err := store.PendingTasks(ctx, q, moveDevicesTask)
	if err != nil || len(tasks) == 0 {
		return movePlan{}, err
	}
	transports, err := store.FreeTransports(ctx, q)
	if err != nil {
		return movePlan{}, err
	}
	pos := map[string]model.Position{}
	stars := make([]string, 0, len(tasks)+len(transports))
	for _, t := range tasks {
		stars = append(stars, auto.StarOf(t.Location))
	}
	for _, t := range transports {
		stars = append(stars, t.Star)
	}
	for _, s := range stars {
		if _, ok := pos[s]; ok {
			continue
		}
		if pos[s], err = store.StarPosition(ctx, q, s); err != nil {
			return movePlan{}, err
		}
	}
	return planMoves(tasks, transports, pos)
}

// assignMoves plans and stores move_devices assignments in one transaction.
func assignMoves(ctx context.Context, db *sql.DB, w io.Writer) error {
	return inTx(ctx, db, func(tx *sql.Tx) error {
		_, err := assignPendingMoves(ctx, tx, w)
		return err
	})
}

// assignPendingMoves gives pending move_devices tasks to free transports
// (planMoves), within tx, and returns the ids of the tasks it assigned.
func assignPendingMoves(ctx context.Context, tx *sql.Tx, w io.Writer) ([]int64, error) {
	p, err := readMovePlan(ctx, tx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	var ids []int64
	for _, a := range p.Assign {
		if err := store.AssignTask(ctx, tx, a.Task.ID, a.Transport.Code, now); err != nil {
			return nil, err
		}
		ids = append(ids, a.Task.ID)
	}
	p.print(w)
	return ids, nil
}

func (p movePlan) print(w io.Writer) {
	fmt.Fprintf(w, "move tasks: %d to assign, %d left pending\n", len(p.Assign), len(p.Waiting))
	for _, a := range p.Assign {
		fmt.Fprintf(w, "  %s %d %s -> %s (%d devices) <- %s %s at %s (capacity %d, %.1f ly)\n",
			moveDevicesTask, a.Task.ID, a.Task.Location, a.Move.Dst, len(a.Move.Devices),
			a.Transport.Type, a.Transport.Code, a.Transport.Location, a.Transport.Capacity, a.Distance)
	}
	for _, t := range p.Waiting {
		fmt.Fprintf(w, "  %s %d %s: pending, no free transport can carry it\n", moveDevicesTask, t.ID, t.Location)
	}
}

// moveDevices is the move_devices task's handler: t.Assignee is a transport
// that carries the task's devices from t.Location (the pickup) to its dst.
// Each call takes one step (decideMove), from the transport as the API
// describes it now. Attaching and detaching are checked by fetching the
// transport again. When the passengers are unloaded it finishes the task
// (unblocking whatever waited for it) and finds the transport its next task;
// both then run straight away.
func moveDevices(ctx context.Context, db store.Reader, t store.Task) (taskChange, error) {
	m, err := parseMove(t)
	if err != nil {
		return nil, err
	}
	s := moveState{Pickup: t.Location, Dst: m.Dst, Devices: m.Devices}
	if s.Transport, err = fetchCarrier(ctx, t.Assignee); err != nil {
		return nil, err
	}
	if s.needsPassengers() {
		for _, d := range m.Devices {
			p, err := fetchMember(ctx, d, "", "")
			if err != nil {
				return nil, err
			}
			s.Passengers = append(s.Passengers, p)
		}
	}
	a, err := decideMove(s)
	if err != nil {
		return nil, err
	}
	tr := s.Transport.Code
	log.Printf("task %d %s %s -> %s (%s): %s", t.ID, t.Task, t.Location, m.Dst, tr, a.Why)

	travel := func(dest string) error {
		if err := store.CheckRelayStar(ctx, db, auto.StarOf(dest)); err != nil {
			return fmt.Errorf("not sending %s to %s: %w", tr, dest, err)
		}
		return api.Travel(ctx, tr, dest)
	}
	switch a.Step {
	case moveWait:
		// Travelling: loaded means on the way to the destination. This also
		// fills in the phase of a move started before phases existed.
		if len(s.Transport.Attached) > 0 {
			return setPhase(t, phaseToDestination), nil
		}
		return setPhase(t, phaseToPickup), nil
	case moveToPickup:
		return setPhase(t, phaseToPickup), travel(t.Location)
	case moveToDst:
		return setPhase(t, phaseToDestination), travel(m.Dst)
	case moveLoad:
		if err := api.AttachDevice(ctx, tr, a.Attach...); err != nil {
			return nil, err
		}
		if err := checkAttached(ctx, tr, m.Devices); err != nil {
			return nil, err
		}
		return setPhase(t, phaseToDestination), travel(m.Dst)
	case moveUnload:
		if err := api.DetachDevice(ctx, tr, s.Transport.Attached...); err != nil {
			return nil, err
		}
		if err := checkAttached(ctx, tr, nil); err != nil {
			return nil, err
		}
		return finishMove(t, s.Transport), nil
	case moveFinish:
		return finishMove(t, s.Transport), nil
	}
	return nil, fmt.Errorf("unknown step %d", a.Step)
}

// finishMove archives the move as done (unblocking what waited for it) and
// gives the transport its next task.
func finishMove(t store.Task, tr carrier) taskChange {
	return func(ctx context.Context, tx *sql.Tx) ([]int64, error) {
		return finishAndNext(ctx, tx, t, tr.Code, tr.Type, auto.StarOf(tr.Location))
	}
}

// fetchCarrier reads a transport from the API.
func fetchCarrier(ctx context.Context, code string) (carrier, error) {
	d, err := fetchDevice(ctx, code)
	if err != nil {
		return carrier{}, err
	}
	c := carrier{
		member: member{Code: code, Type: d.DeviceType, Location: d.Location, Status: d.Status,
			AttachedTo: d.AttachedToDeviceCode, Destination: d.Travel.FinalDestination},
		Free: d.AttachCapacity - d.AttachUsed,
	}
	for _, a := range d.AttachedDevices {
		c.Attached = append(c.Attached, a.DeviceCode)
	}
	return c, nil
}

// checkAttached fetches transport again and checks it carries exactly want.
func checkAttached(ctx context.Context, transport string, want []string) error {
	c, err := fetchCarrier(ctx, transport)
	if err != nil {
		return err
	}
	got, w := slices.Sorted(slices.Values(c.Attached)), slices.Sorted(slices.Values(want))
	if !slices.Equal(got, w) {
		return fmt.Errorf("%s carries [%s], want [%s]", transport, strings.Join(got, " "), strings.Join(w, " "))
	}
	return nil
}

// move_devices phases.
const (
	phaseToPickup      = "to_pickup"
	phaseToDestination = "to_destination"
)

type moveStep int

const (
	moveWait     moveStep = iota // travelling: wait until it arrives
	moveToPickup                 // travel to the pickup point
	moveLoad                     // attach Attach, check, travel to Dst
	moveToDst                    // everything's aboard: travel to Dst
	moveUnload                   // detach everything, check, finish
	moveFinish                   // already unloaded at Dst: finish
)

type moveAction struct {
	Step   moveStep
	Attach []string // moveLoad: the passengers still to attach
	Why    string   // for the log
}

// carrier is a transport as the API describes it now.
type carrier struct {
	member
	Attached []string // codes of the devices attached to it
	Free     int      // attach_capacity - attach_used
}

type moveState struct {
	Pickup, Dst string
	Devices     []string // the passengers
	Transport   carrier
	// Passengers as the API describes them now, in Devices order. Only
	// needed (and fetched) when the transport is at Pickup, or at Dst with
	// nothing attached; nil otherwise.
	Passengers []member
}

// needsPassengers reports whether decideMove will need s.Passengers.
func (s moveState) needsPassengers() bool {
	t := s.Transport
	return t.Location != "" && (t.Location == s.Pickup || (t.Location == s.Dst && len(t.Attached) == 0))
}

// decideMove is the move_devices task's next step:
//
//	travelling? wait
//	at the destination: passengers attached? detach them and finish
//	  nothing attached, passengers all there? finish
//	every passenger attached? travel to the destination
//	at the pickup: attach the passengers not yet attached (all must be
//	  there, not attached elsewhere, and fit), then travel to the destination
//	nothing attached? travel to the pickup
//
// Anything else is an error: carrying a device that isn't a passenger, some
// passengers attached away from the pickup, a passenger not at the pickup,
// or not enough room.
func decideMove(s moveState) (moveAction, error) {
	t := s.Transport
	if t.Location == "" {
		return moveAction{Step: moveWait, Why: fmt.Sprintf("waiting: %s is %s (destination %q)", t.Code, t.Status, t.Destination)}, nil
	}
	for _, a := range t.Attached {
		if !slices.Contains(s.Devices, a) {
			return moveAction{}, fmt.Errorf("%s is carrying %s, which isn't one of the devices to move", t.Code, a)
		}
	}
	all := len(t.Attached) == len(s.Devices)
	if s.needsPassengers() && len(s.Passengers) != len(s.Devices) {
		return moveAction{}, fmt.Errorf("have %d passengers' details, want %d", len(s.Passengers), len(s.Devices))
	}

	if t.Location == s.Dst {
		if len(t.Attached) > 0 {
			if !all {
				return moveAction{}, fmt.Errorf("%s reached %s carrying only %s of %s", t.Code, s.Dst, strings.Join(t.Attached, " "), strings.Join(s.Devices, " "))
			}
			return moveAction{Step: moveUnload, Why: "at " + s.Dst + ": unloading"}, nil
		}
		for _, p := range s.Passengers {
			if p.Location != s.Dst || p.AttachedTo != "" {
				return moveAction{}, fmt.Errorf("%s is at %s with nothing attached, but %s is at %q (attached to %q)", t.Code, s.Dst, p.Code, p.Location, p.AttachedTo)
			}
		}
		return moveAction{Step: moveFinish, Why: "passengers already unloaded at " + s.Dst}, nil
	}
	if all {
		return moveAction{Step: moveToDst, Why: "everything aboard: heading to " + s.Dst}, nil
	}
	if t.Location == s.Pickup {
		var attach []string
		for _, p := range s.Passengers {
			if p.AttachedTo == t.Code {
				continue
			}
			if p.Location != s.Pickup || p.AttachedTo != "" {
				return moveAction{}, fmt.Errorf("passenger %s is at %q (attached to %q, status %s), not waiting at %s", p.Code, p.Location, p.AttachedTo, p.Status, s.Pickup)
			}
			attach = append(attach, p.Code)
		}
		if len(attach) > t.Free {
			return moveAction{}, fmt.Errorf("%s has room for %d, but %d passengers to attach", t.Code, t.Free, len(attach))
		}
		return moveAction{Step: moveLoad, Attach: attach, Why: fmt.Sprintf("at %s: attaching %s, then heading to %s", s.Pickup, strings.Join(attach, " "), s.Dst)}, nil
	}
	if len(t.Attached) > 0 {
		return moveAction{}, fmt.Errorf("%s is at %s carrying only some passengers (%s)", t.Code, t.Location, strings.Join(t.Attached, " "))
	}
	return moveAction{Step: moveToPickup, Why: "heading to " + s.Pickup}, nil
}
