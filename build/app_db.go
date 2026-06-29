package main

// app_db.go — app-specific database migrations for food-scaner (food scanner).
// initDB() (db.go) calls appMigrate() after the core users table is ready.
//
// Three tables back the whole app (see skeleton/db.md):
//   meal      — today's eaten meals (the donut SUMs these; the history table lists them)
//   favorite  — saved meal templates that PERSIST (never day-swept)
//   ai_usage  — one AI-request counter per user per MSK day (resets daily)
//
// Per-user scan economy lives on the users row as three extra columns:
//   role             free | tester | pro
//   free_scans_left  lifetime free AI ops for a `free` user (default 3)
//   daily_limit      AI ops/day for a `tester` (default 10)

func appMigrate() error {
	// Extend the core users table. ALTER fallbacks are idempotent: harmless if the
	// column already exists (db.go owns the base table, so we add ours here).
	for _, alter := range []string{
		`ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'free'`,
		`ALTER TABLE users ADD COLUMN free_scans_left INTEGER NOT NULL DEFAULT 3`,
		`ALTER TABLE users ADD COLUMN daily_limit INTEGER NOT NULL DEFAULT 10`,
	} {
		db.Exec(alter) //nolint:errcheck — ok if column already exists
	}

	_, err := db.Exec(`
		-- today's eaten meals. The donut SUMs these; the history table lists them.
		CREATE TABLE IF NOT EXISTS meal (
			id        INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id   INTEGER NOT NULL REFERENCES users(id),
			day       TEXT    NOT NULL,                          -- 'YYYY-MM-DD' MSK day (reset key)
			name      TEXT    NOT NULL,
			kcal      INTEGER NOT NULL,
			grams     INTEGER NOT NULL DEFAULT 0,
			prot      REAL    NOT NULL DEFAULT 0,
			fat       REAL    NOT NULL DEFAULT 0,
			carb      REAL    NOT NULL DEFAULT 0,
			eaten_at  TEXT    NOT NULL DEFAULT (datetime('now'))  -- newest-first ordering
		);
		CREATE INDEX IF NOT EXISTS idx_meal_user_day ON meal (user_id, day, eaten_at DESC);

		-- favorites library — saved meal templates. These PERSIST (no day, never swept).
		CREATE TABLE IF NOT EXISTS favorite (
			id      INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL REFERENCES users(id),
			name    TEXT    NOT NULL,
			kcal    INTEGER NOT NULL,
			grams   INTEGER NOT NULL DEFAULT 0,
			prot    REAL    NOT NULL DEFAULT 0,
			fat     REAL    NOT NULL DEFAULT 0,
			carb    REAL    NOT NULL DEFAULT 0,
			UNIQUE (user_id, name)                               -- star upserts by name → no dupes
		);

		-- AI usage — one counter per user per MSK day. Photo + text count the same.
		CREATE TABLE IF NOT EXISTS ai_usage (
			user_id INTEGER NOT NULL,
			day     TEXT    NOT NULL,                            -- 'YYYY-MM-DD' MSK day
			uses    INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (user_id, day)
		);
	`)
	return err
}
