package store

import "encoding/json"

// ReclassifyUnreleased moves an app's production data whose build was never released on the App Store to
// sandbox (TestFlight). released lists the released "version|build" pairs. Returns the number of reclassified
// events. Does nothing when the list is empty.
func (s *Store) ReclassifyUnreleased(app string, released []string) (int64, error) {
	if len(released) == 0 {
		return 0, nil
	}
	rel, _ := json.Marshal(released)
	return s.reclassifyProduction(app,
		`(COALESCE(app_version, '') || '|' || COALESCE(build, '')) NOT IN (SELECT value FROM json_each(?))`, string(rel))
}

// ReclassifyMixedInstalls moves the production data of installs that have also sent sandbox / xcode data to sandbox.
// Real App Store users never produce sandbox / xcode data; an install with both can only be a development device
// whose TestFlight build failed to get AppTransaction and fell back to production (and that build was also
// released, so ReclassifyUnreleased does not catch it).
func (s *Store) ReclassifyMixedInstalls(app string) (int64, error) {
	// An uncorrelated subquery is evaluated once; a correlated one (x.app = events.app) is re-evaluated per row
	// and stalls startup for over ten seconds
	return s.reclassifyProduction(app, `install_id IN (
		SELECT install_id FROM events WHERE app = ? AND env IN ('sandbox', 'xcode'))`, app)
}

// reclassifyProduction moves an app's production events matching cond to sandbox and returns the number of
// reclassified events.
//
// events and purchase_attempts are updated row by row; daily_active is recomputed from events for the affected
// installs and days (same rules as WriteBatches); installs.env takes the environment of the install's latest event.
func (s *Store) reclassifyProduction(app, cond string, condArgs ...any) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	where := `app = ? AND env = 'production' AND ` + cond
	args := append([]any{app}, condArgs...)

	// Affected (install, day) pairs
	rows, err := tx.Query(`SELECT DISTINCT install_id, day FROM events WHERE `+where, args...)
	if err != nil {
		return 0, err
	}
	days := map[string][]string{}
	for rows.Next() {
		var id, day string
		if err := rows.Scan(&id, &day); err != nil {
			rows.Close()
			return 0, err
		}
		days[id] = append(days[id], day)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(days) == 0 {
		return 0, nil
	}

	res, err := tx.Exec(`UPDATE events SET env = 'sandbox' WHERE `+where, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()

	for id, ds := range days {
		dj, _ := json.Marshal(ds)
		// Purchase attempts follow the environment of their purchase.started event
		if _, err := tx.Exec(`
			UPDATE purchase_attempts SET env = 'sandbox'
			WHERE app = ? AND install_id = ? AND env = 'production' AND token IN (
				SELECT lower(json_extract(params, '$.token')) FROM events
				WHERE app = ? AND install_id = ? AND name = 'purchase.started' AND env = 'sandbox')`,
			app, id, app, id); err != nil {
			return 0, err
		}
		// Daily active: delete the rows for these days and recompute them from events
		if _, err := tx.Exec(`DELETE FROM daily_active WHERE app = ? AND install_id = ? AND day IN (SELECT value FROM json_each(?))`,
			app, id, string(dj)); err != nil {
			return 0, err
		}
		if _, err := tx.Exec(`
			INSERT INTO daily_active (app, env, day, install_id, sessions, duration_s, events)
			SELECT app, env, day, install_id,
				SUM(name = 'session.started'),
				SUM(CASE WHEN name = 'session.ended'
					THEN MAX(0, MIN(COALESCE(CAST(json_extract(params, '$.duration_s') AS INTEGER), 0), 86400)) ELSE 0 END),
				COUNT(*)
			FROM events
			WHERE app = ? AND install_id = ? AND day IN (SELECT value FROM json_each(?))
			GROUP BY app, env, day, install_id`,
			app, id, string(dj)); err != nil {
			return 0, err
		}
		if _, err := tx.Exec(`
			UPDATE installs SET env = (
				SELECT env FROM events e WHERE e.app = installs.app AND e.install_id = installs.install_id
				ORDER BY ts DESC LIMIT 1)
			WHERE app = ? AND install_id = ?`, app, id); err != nil {
			return 0, err
		}
	}
	return n, tx.Commit()
}
