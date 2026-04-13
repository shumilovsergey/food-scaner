package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "modernc.org/sqlite"
)

var db *sql.DB

func initDB() {
	path := os.Getenv("DB_PATH")
	if path == "" {
		path = "./data/foodscaner.db"
	}

	var err error
	db, err = sql.Open("sqlite", path)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			auth_id         TEXT UNIQUE NOT NULL,
			method          TEXT NOT NULL DEFAULT '',
			username        TEXT NOT NULL DEFAULT '',
			role            TEXT NOT NULL DEFAULT 'free',
			daily_limit     INTEGER NOT NULL DEFAULT 10,
			free_scans_left INTEGER NOT NULL DEFAULT 3,
			owned_scans     INTEGER NOT NULL DEFAULT 0,
			pro_until       TEXT,
			created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS scans (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id    INTEGER NOT NULL REFERENCES users(id),
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		log.Fatalf("migrate db: %v", err)
	}

	// Migrations for existing databases — safe to run repeatedly
	for _, m := range []string{
		`ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'free'`,
		`ALTER TABLE users ADD COLUMN free_scans_left INTEGER NOT NULL DEFAULT 3`,
		`ALTER TABLE users ADD COLUMN owned_scans INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE users ADD COLUMN pro_until TEXT`,
		// Carry over previously-approved users as testers
		`UPDATE users SET role='tester' WHERE approved=1 AND role='free'`,
	} {
		db.Exec(m) // ignore errors (column already exists etc.)
	}
}

// ── User ─────────────────────────────────────────────────────────────────

type User struct {
	ID            int64
	AuthID        string
	Method        string
	Username      string
	Role          string // "free" | "tester" | "pro"
	DailyLimit    int
	FreeScansLeft int
	OwnedScans    int
	ProUntil      *time.Time
}

func scanUser(row *sql.Row) (User, error) {
	var u User
	var proUntilStr sql.NullString
	err := row.Scan(
		&u.ID, &u.AuthID, &u.Method, &u.Username,
		&u.Role, &u.DailyLimit, &u.FreeScansLeft, &u.OwnedScans, &proUntilStr,
	)
	if err != nil {
		return User{}, err
	}
	if proUntilStr.Valid && proUntilStr.String != "" {
		t, err := time.Parse("2006-01-02", proUntilStr.String)
		if err == nil {
			u.ProUntil = &t
		}
	}
	return u, nil
}

const userFields = `id, auth_id, method, username, role, daily_limit, free_scans_left, owned_scans, pro_until`

func upsertUser(authID, method, username string) (User, error) {
	_, err := db.Exec(`
		INSERT INTO users (auth_id, method, username)
		VALUES (?, ?, ?)
		ON CONFLICT(auth_id) DO UPDATE SET method=excluded.method, username=excluded.username
	`, authID, method, username)
	if err != nil {
		return User{}, err
	}
	return getUserByAuthID(authID)
}

func getUserByAuthID(authID string) (User, error) {
	return scanUser(db.QueryRow(`SELECT `+userFields+` FROM users WHERE auth_id = ?`, authID))
}

func getUserByID(id int64) (User, error) {
	return scanUser(db.QueryRow(`SELECT `+userFields+` FROM users WHERE id = ?`, id))
}

// ── Scan access ───────────────────────────────────────────────────────────

type ScanAccess struct {
	Allowed bool
	Reason  string // "pro" | "tester" | "owned" | "free" | "no_scans" | "daily_limit"
}

func canScan(u User) (ScanAccess, error) {
	// PRO — unlimited while active
	if u.Role == "pro" && u.ProUntil != nil && u.ProUntil.After(time.Now()) {
		return ScanAccess{true, "pro"}, nil
	}
	// Tester — daily cap
	if u.Role == "tester" {
		count, err := countTodayScans(u.ID)
		if err != nil {
			return ScanAccess{}, err
		}
		if count < u.DailyLimit {
			return ScanAccess{true, "tester"}, nil
		}
		return ScanAccess{false, "daily_limit"}, nil
	}
	// Owned scan pack
	if u.OwnedScans > 0 {
		return ScanAccess{true, "owned"}, nil
	}
	// Free quota
	if u.FreeScansLeft > 0 {
		return ScanAccess{true, "free"}, nil
	}
	return ScanAccess{false, "no_scans"}, nil
}

func consumeScan(userID int64, reason string) {
	insertScan(userID)
	switch reason {
	case "owned":
		db.Exec(`UPDATE users SET owned_scans = owned_scans - 1 WHERE id = ?`, userID)
	case "free":
		db.Exec(`UPDATE users SET free_scans_left = free_scans_left - 1 WHERE id = ?`, userID)
	}
}

// ── Scans table ───────────────────────────────────────────────────────────

func countTodayScans(userID int64) (int, error) {
	var n int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM scans
		WHERE user_id = ? AND DATE(created_at) = DATE('now')
	`, userID).Scan(&n)
	return n, err
}

func countTotalScans(userID int64) (int, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM scans WHERE user_id = ?`, userID).Scan(&n)
	return n, err
}

func insertScan(userID int64) {
	db.Exec(`INSERT INTO scans (user_id) VALUES (?)`, userID)
}

// ── Admin helpers ─────────────────────────────────────────────────────────

func listUsers() ([]User, error) {
	rows, err := db.Query(`SELECT ` + userFields + ` FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		var u User
		var proUntilStr sql.NullString
		rows.Scan(&u.ID, &u.AuthID, &u.Method, &u.Username,
			&u.Role, &u.DailyLimit, &u.FreeScansLeft, &u.OwnedScans, &proUntilStr)
		if proUntilStr.Valid {
			t, _ := time.Parse("2006-01-02", proUntilStr.String)
			u.ProUntil = &t
		}
		users = append(users, u)
	}
	return users, nil
}

func proUntilStr(u User) string {
	if u.ProUntil == nil {
		return "—"
	}
	return u.ProUntil.Format("2006-01-02")
}

func userStatusStr(u User) string {
	switch u.Role {
	case "pro":
		return fmt.Sprintf("PRO until %s", proUntilStr(u))
	case "tester":
		return fmt.Sprintf("tester (%d/day)", u.DailyLimit)
	default:
		return fmt.Sprintf("free (%d left, %d owned)", u.FreeScansLeft, u.OwnedScans)
	}
}
