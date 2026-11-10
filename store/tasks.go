package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Task is a row of the tasks table (the columns we use so far).
type Task struct {
	ID       int64
	Task     string
	Location string
	Priority int
	Status   string
	Phase    string // where within the task we are, set by its handler; "" until started
	Assignee string // "" if unassigned
	Details  string // JSON, "" if none
}

const taskColumns = `id, task, coalesce(location, ''), priority, status, coalesce(phase, ''), coalesce(assignee, ''), coalesce(details, '')`

// TasksOfType lists every task of the given type, failed ones included.
func TasksOfType(ctx context.Context, q Reader, task string) ([]Task, error) {
	return tasks(ctx, q, `SELECT `+taskColumns+` FROM tasks WHERE task = ? ORDER BY id`, task)
}

// TaskByID reads task id; ok is false if there's no such task (it may have
// been archived).
func TaskByID(ctx context.Context, q Reader, id int64) (t Task, ok bool, err error) {
	ts, err := tasks(ctx, q, `SELECT `+taskColumns+` FROM tasks WHERE id = ?`, id)
	if err != nil || len(ts) == 0 {
		return Task{}, false, err
	}
	return ts[0], true, nil
}

// AssignedTasks lists the tasks that are assigned (so not pending, blocked or
// failed), highest priority (lowest number) first, then oldest first.
func AssignedTasks(ctx context.Context, q Reader) ([]Task, error) {
	return tasks(ctx, q, `SELECT `+taskColumns+` FROM tasks WHERE status = 'assigned' ORDER BY priority, id`)
}

func tasks(ctx context.Context, q Reader, query string, args ...any) ([]Task, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ts []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.Task, &t.Location, &t.Priority, &t.Status, &t.Phase, &t.Assignee, &t.Details); err != nil {
			return nil, err
		}
		ts = append(ts, t)
	}
	return ts, rows.Err()
}

// AddTask inserts a pending task and returns its id. details is JSON, or ""
// for none.
func AddTask(ctx context.Context, q Queryer, task, location, details string, now time.Time) (int64, error) {
	var id int64
	err := q.QueryRowContext(ctx, `INSERT INTO tasks (task, location, details, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?) RETURNING id`, task, null(location), null(details), now.UnixMilli(), now.UnixMilli()).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("adding %s task at %s: %w", task, location, err)
	}
	return id, nil
}

// PendingTasks lists the pending, unassigned tasks of the given types,
// highest priority first, then oldest.
func PendingTasks(ctx context.Context, q Reader, types ...string) ([]Task, error) {
	if len(types) == 0 {
		return nil, nil
	}
	query := `SELECT ` + taskColumns + ` FROM tasks WHERE status = 'pending' AND assignee IS NULL AND task IN (?` +
		strings.Repeat(", ?", len(types)-1) + `) ORDER BY priority, id`
	args := make([]any, len(types))
	for i, t := range types {
		args[i] = t
	}
	return tasks(ctx, q, query, args...)
}

// FinishTask moves task id to archived_tasks as done. Deleting it removes its
// blockers rows, and any blocked task left with no blockers becomes assigned
// (or pending, if it has no assignee). It returns the ids of the tasks it
// unblocked.
//
// TODO: a failed task must not unblock its dependents like this; it should
// wake them to decide (design/tasks.md). There's no FailTask yet.
func FinishTask(ctx context.Context, tx *sql.Tx, id int64, now time.Time) ([]int64, error) {
	res, err := tx.ExecContext(ctx, `INSERT INTO archived_tasks
		(id, task, location, priority, status, assignee, details, error, created_at, updated_at, finished_at)
		SELECT id, task, location, priority, 'done', assignee, details, error, created_at, ?, ?
		FROM tasks WHERE id = ? AND status = 'assigned'`, now.UnixMilli(), now.UnixMilli(), id)
	if err != nil {
		return nil, fmt.Errorf("archiving task %d: %w", id, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return nil, err
	} else if n != 1 {
		return nil, fmt.Errorf("archiving task %d: no assigned task with that id", id)
	}
	var blocked []int64
	rows, err := tx.QueryContext(ctx, `SELECT task FROM blockers WHERE dep = ?`, id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var t int64
		if err := rows.Scan(&t); err != nil {
			rows.Close()
			return nil, err
		}
		blocked = append(blocked, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, id); err != nil {
		return nil, fmt.Errorf("deleting task %d: %w", id, err)
	}
	var unblocked []int64
	for _, t := range blocked {
		res, err := tx.ExecContext(ctx, `UPDATE tasks
			SET status = CASE WHEN assignee IS NULL THEN 'pending' ELSE 'assigned' END, updated_at = ?
			WHERE id = ? AND status = 'blocked' AND NOT EXISTS (SELECT 1 FROM blockers WHERE task = ?)`,
			now.UnixMilli(), t, t)
		if err != nil {
			return nil, err
		}
		if n, _ := res.RowsAffected(); n == 1 {
			unblocked = append(unblocked, t)
		}
	}
	return unblocked, nil
}

// Block makes task wait for dep: it adds the blockers row and sets task
// blocked (it keeps its assignee). task must be assigned or pending.
func Block(ctx context.Context, ex Execer, task, dep int64, now time.Time) error {
	if _, err := ex.ExecContext(ctx, `INSERT INTO blockers (task, dep) VALUES (?, ?)`, task, dep); err != nil {
		return fmt.Errorf("blocking task %d on %d: %w", task, dep, err)
	}
	res, err := ex.ExecContext(ctx, `UPDATE tasks SET status = 'blocked', updated_at = ?
		WHERE id = ? AND status IN ('assigned', 'pending')`, now.UnixMilli(), task)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n != 1 {
		return fmt.Errorf("blocking task %d: no assigned or pending task with that id", task)
	}
	return nil
}

// AssignTask gives the pending, unassigned task id to assignee. Anything else
// is an error.
func AssignTask(ctx context.Context, ex Execer, id int64, assignee string, now time.Time) error {
	res, err := ex.ExecContext(ctx, `UPDATE tasks SET status = 'assigned', assignee = ?, updated_at = ?
		WHERE id = ? AND status = 'pending' AND assignee IS NULL`, assignee, now.UnixMilli(), id)
	if err != nil {
		return fmt.Errorf("assigning task %d to %s: %w", id, assignee, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n != 1 {
		return fmt.Errorf("assigning task %d to %s: no pending, unassigned task with that id", id, assignee)
	}
	return nil
}

// UnsurveyedRelayStars lists the stars with a relaying ftl_relay whose planets
// and moons aren't all scanned. A relay star whose system we haven't read
// (no scan counts) is an error.
func UnsurveyedRelayStars(ctx context.Context, q Reader) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT r.star, s.designation IS NOT NULL, s.planet_count IS NOT NULL AND s.moon_count IS NOT NULL
		FROM relay_stars r LEFT JOIN stars s ON s.designation = r.star
		WHERE NOT coalesce(s.is_fully_scanned, 0) ORDER BY r.star`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var stars []string
	for rows.Next() {
		var star string
		var known, read bool
		if err := rows.Scan(&star, &known, &read); err != nil {
			return nil, err
		}
		if !known {
			return nil, fmt.Errorf("relay star %s isn't in the stars table", star)
		}
		if !read {
			return nil, fmt.Errorf("relay star %s: we haven't read its system (no planet or moon count)", star)
		}
		stars = append(stars, star)
	}
	return stars, rows.Err()
}

// MinSurveyDrones is how many survey_drones a survey team needs.
const MinSurveyDrones = 5

// SurveyTeam is an ami_survey_controller and the survey_drones it controls.
type SurveyTeam struct {
	Controller, Location, Star string
	Drones                     int
}

// FreeSurveyTeams lists the ami_survey_controllers that control at least
// MinSurveyDrones survey_drones, have a location (aren't travelling or
// stowed), control no belt_surveyor (belt search teams stay put) and aren't
// any task's assignee. The drones are assumed to be with their controller.
func FreeSurveyTeams(ctx context.Context, q Reader) ([]SurveyTeam, error) {
	rows, err := q.QueryContext(ctx, `SELECT c.device_code, c.location, c.star, count(*)
		FROM devices c JOIN devices d ON d.controller = c.device_code AND d.device_type = 'survey_drone'
		WHERE c.device_type = 'ami_survey_controller' AND c.location IS NOT NULL
			AND NOT EXISTS (SELECT 1 FROM devices b WHERE b.controller = c.device_code AND b.device_type = 'belt_surveyor')
			AND NOT EXISTS (SELECT 1 FROM tasks t WHERE t.assignee = c.device_code)
		GROUP BY c.device_code HAVING count(*) >= ?
		ORDER BY c.device_code`, MinSurveyDrones)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var teams []SurveyTeam
	for rows.Next() {
		var t SurveyTeam
		if err := rows.Scan(&t.Controller, &t.Location, &t.Star, &t.Drones); err != nil {
			return nil, err
		}
		teams = append(teams, t)
	}
	return teams, rows.Err()
}

// EntryPoint is star's entry point. A star that isn't in the store, or whose
// entry point we don't know, is an error.
func EntryPoint(ctx context.Context, q Queryer, star string) (string, error) {
	var ep sql.NullString
	err := q.QueryRowContext(ctx, `SELECT entry_point FROM stars WHERE designation = ?`, star).Scan(&ep)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("star %s isn't in the store", star)
	}
	if err != nil {
		return "", err
	}
	if !ep.Valid || ep.String == "" {
		return "", fmt.Errorf("star %s: entry point not known", star)
	}
	return ep.String, nil
}

// IsFullyScanned reports whether every planet and moon at star is scanned.
// monitor keeps the counts current during a survey: the final
// ami.survey.digest marks the system done (SurveyProgress), and the
// controller's directive.completed reads /locations/STAR again.
func IsFullyScanned(ctx context.Context, q Queryer, star string) (bool, error) {
	var full sql.NullBool
	err := q.QueryRowContext(ctx, `SELECT is_fully_scanned FROM stars WHERE designation = ?`, star).Scan(&full)
	if errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("star %s isn't in the store", star)
	}
	if err != nil {
		return false, err
	}
	if !full.Valid {
		return false, fmt.Errorf("star %s: we haven't read its system (no scan counts)", star)
	}
	return full.Bool, nil
}

// ControlledBy lists the devices whose controller is controller, by code.
func ControlledBy(ctx context.Context, q Reader, controller string) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT device_code FROM devices WHERE controller = ? ORDER BY device_code`, controller)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var codes []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		codes = append(codes, c)
	}
	return codes, rows.Err()
}

// Transport is a device that can carry others between systems.
type Transport struct {
	Code, Type, Location, Star string
	Capacity                   int // attach_capacity, from its blueprint
}

// FreeTransports lists the devices with both surge and attach in their
// features that are free to take a move_devices task: they have a location
// (aren't travelling), aren't attached or stowed, carry nothing, aren't any
// task's assignee or claimed by one, and aren't among the devices a
// move_devices task is to move. A transport type with no blueprint is an
// error.
func FreeTransports(ctx context.Context, q Reader) ([]Transport, error) {
	rows, err := q.QueryContext(ctx, `SELECT d.device_code, d.device_type, d.location, d.star, b.attach_capacity
		FROM devices d LEFT JOIN blueprints b USING (device_type)
		WHERE instr(',' || coalesce(d.features, '') || ',', ',surge,') > 0
			AND instr(',' || coalesce(d.features, '') || ',', ',attach,') > 0
			AND d.location IS NOT NULL AND d.attached_to IS NULL AND d.stowed_in IS NULL
			AND NOT EXISTS (SELECT 1 FROM devices a WHERE a.attached_to = d.device_code)
			AND NOT EXISTS (SELECT 1 FROM tasks t WHERE t.assignee = d.device_code)
			AND NOT EXISTS (SELECT 1 FROM claimed_devices c WHERE c.device = d.device_code)
			AND NOT EXISTS (SELECT 1 FROM tasks t, json_each(t.details, '$.devices') m
				WHERE t.task = 'move_devices' AND m.value = d.device_code)
		ORDER BY d.device_code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ts []Transport
	for rows.Next() {
		var t Transport
		var capacity sql.NullInt64
		if err := rows.Scan(&t.Code, &t.Type, &t.Location, &t.Star, &capacity); err != nil {
			return nil, err
		}
		if !capacity.Valid {
			return nil, fmt.Errorf("transport %s: no blueprint for %s", t.Code, t.Type)
		}
		t.Capacity = int(capacity.Int64)
		ts = append(ts, t)
	}
	return ts, rows.Err()
}

// AttachCapacity is how many devices one deviceType can carry, from its
// blueprint. A type with no blueprint is an error.
func AttachCapacity(ctx context.Context, q Queryer, deviceType string) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT attach_capacity FROM blueprints WHERE device_type = ?`, deviceType).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("no blueprint for %s", deviceType)
	}
	return n, err
}

// CheckRelayStar is an error unless star has a relaying ftl_relay (anything
// sent to a system without one can't be controlled).
func CheckRelayStar(ctx context.Context, q Queryer, star string) error {
	var ok bool
	if err := q.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM relay_stars WHERE star = ?)`, star).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%s has no relaying ftl_relay", star)
	}
	return nil
}

// SurveyPhase shows a survey's progress in the phase of the assigned
// survey_system task at star whose assignee is controller, e.g.
// "surveying 14/31" (planets and moons together). It leaves updated_at alone,
// so that still says when the task entered its phase. No such task (a
// survey started by hand) is fine.
func SurveyPhase(ctx context.Context, ex Execer, star, controller string, scanned, total int) error {
	_, err := ex.ExecContext(ctx, `UPDATE tasks SET phase = ?
		WHERE task = 'survey_system' AND location = ? AND assignee = ? AND status = 'assigned'`,
		fmt.Sprintf("surveying %d/%d", scanned, total), star, controller)
	return err
}

// SetPhase records where within assigned task id we are (e.g. to_pickup).
func SetPhase(ctx context.Context, ex Execer, id int64, phase string, now time.Time) error {
	res, err := ex.ExecContext(ctx, `UPDATE tasks SET phase = ?, updated_at = ? WHERE id = ? AND status = 'assigned'`,
		phase, now.UnixMilli(), id)
	if err != nil {
		return fmt.Errorf("setting task %d's phase to %s: %w", id, phase, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n != 1 {
		return fmt.Errorf("setting task %d's phase to %s: no assigned task with that id", id, phase)
	}
	return nil
}

// ControllerOf is device's controller, or "" if it has none or isn't in the
// store.
func ControllerOf(ctx context.Context, q Queryer, device string) (string, error) {
	var c sql.NullString
	err := q.QueryRowContext(ctx, `SELECT controller FROM devices WHERE device_code = ?`, device).Scan(&c)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return c.String, err
}
