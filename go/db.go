package main

import (
	"database/sql"
	"log"
	"os"

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
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			auth_id     TEXT UNIQUE NOT NULL,
			method      TEXT NOT NULL,
			username    TEXT NOT NULL DEFAULT '',
			approved    INTEGER NOT NULL DEFAULT 0,
			daily_limit INTEGER NOT NULL DEFAULT 10,
			created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
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
}

type User struct {
	ID         int64
	AuthID     string
	Method     string
	Username   string
	Approved   bool
	DailyLimit int
}

// upsertUser inserts or updates a user by auth_id, returns the user row.
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
	var u User
	err := db.QueryRow(`
		SELECT id, auth_id, method, username, approved, daily_limit
		FROM users WHERE auth_id = ?
	`, authID).Scan(&u.ID, &u.AuthID, &u.Method, &u.Username, &u.Approved, &u.DailyLimit)
	return u, err
}

func getUserByID(id int64) (User, error) {
	var u User
	err := db.QueryRow(`
		SELECT id, auth_id, method, username, approved, daily_limit
		FROM users WHERE id = ?
	`, id).Scan(&u.ID, &u.AuthID, &u.Method, &u.Username, &u.Approved, &u.DailyLimit)
	return u, err
}

func countTodayScans(userID int64) (int, error) {
	var n int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM scans
		WHERE user_id = ? AND DATE(created_at) = DATE('now')
	`, userID).Scan(&n)
	return n, err
}

func insertScan(userID int64) error {
	_, err := db.Exec(`INSERT INTO scans (user_id) VALUES (?)`, userID)
	return err
}

func listUsers() ([]User, error) {
	rows, err := db.Query(`
		SELECT id, auth_id, method, username, approved, daily_limit
		FROM users ORDER BY id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		var u User
		rows.Scan(&u.ID, &u.AuthID, &u.Method, &u.Username, &u.Approved, &u.DailyLimit)
		users = append(users, u)
	}
	return users, nil
}
