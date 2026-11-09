package store

import (
	"context"
	"fmt"
	"time"
)

// Task is a row of the tasks table (the columns we use so far).
type Task struct {
	ID       int64
	Task     string
	Location string
	Status   string
	Assignee string // "" if unassigned
}

// TasksOfType lists every task of the given type, failed ones included.
func TasksOfType(ctx context.Context, q Reader, task string) ([]Task, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, task, coalesce(location, ''), status, coalesce(assignee, '')
		FROM tasks WHERE task = ? ORDER BY id`, task)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ts []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.Task, &t.Location, &t.Status, &t.Assignee); err != nil {
			return nil, err
		}
		ts = append(ts, t)
	}
	return ts, rows.Err()
}

// AddTask inserts a pending task and returns its id.
func AddTask(ctx context.Context, q Queryer, task, location string, now time.Time) (int64, error) {
	var id int64
	err := q.QueryRowContext(ctx, `INSERT INTO tasks (task, location, created_at, updated_at)
		VALUES (?, ?, ?, ?) RETURNING id`, task, null(location), now.UnixMilli(), now.UnixMilli()).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("adding %s task at %s: %w", task, location, err)
	}
	return id, nil
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
