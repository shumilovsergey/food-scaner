package main

// store.go — food-scaner app data layer (meals, favorites, AI usage, scan economy).
// Everything is user-scoped. The MSK day (UTC+3) is the reset key: meals and the
// AI counter belong to a calendar day and are swept lazily on access (no cron).
// See skeleton/db.md for the model.

import (
	"database/sql"
	"fmt"
)

// mskDayExpr is the SQL for "today" in MSK (UTC+3). SQLite has no date type, so
// the day is stored as ISO 'YYYY-MM-DD' text which sorts/compares correctly.
const mskDayExpr = `date('now','+3 hours')`

type Meal struct {
	ID    int64   `json:"id"`
	Name  string  `json:"name"`
	Kcal  int     `json:"kcal"`
	Grams int     `json:"grams"`
	Prot  float64 `json:"prot"`
	Fat   float64 `json:"fat"`
	Carb  float64 `json:"carb"`
	Time  string  `json:"time"` // 'HH:MM' MSK, for the history row
	Fav   bool    `json:"fav"`  // derived: a favorite with the same name exists
}

type Favorite struct {
	ID    int64   `json:"id"`
	Name  string  `json:"name"`
	Kcal  int     `json:"kcal"`
	Grams int     `json:"grams"`
	Prot  float64 `json:"prot"`
	Fat   float64 `json:"fat"`
	Carb  float64 `json:"carb"`
}

// MealInput is the writable payload shared by meals and favorites.
type MealInput struct {
	Name  string  `json:"name"`
	Kcal  int     `json:"kcal"`
	Grams int     `json:"grams"`
	Prot  float64 `json:"prot"`
	Fat   float64 `json:"fat"`
	Carb  float64 `json:"carb"`
}

// Status is the per-user scan economy, stored on the users row.
type Status struct {
	Role          string `json:"role"` // free | tester | pro
	FreeScansLeft int    `json:"freeScansLeft"`
	DailyLimit    int    `json:"dailyLimit"`
}

// mskDay returns today's MSK calendar day as 'YYYY-MM-DD'.
func mskDay() string {
	var d string
	db.QueryRow(`SELECT ` + mskDayExpr).Scan(&d) //nolint:errcheck
	return d
}

// sweepOldDays clears one user's stale rows (everything not from today's MSK day).
// Cheap, user-scoped, indexed, self-healing — favorites are never touched.
func sweepOldDays(uid int64) {
	db.Exec(`DELETE FROM meal     WHERE user_id=? AND day<>`+mskDayExpr, uid)     //nolint:errcheck
	db.Exec(`DELETE FROM ai_usage WHERE user_id=? AND day<>`+mskDayExpr, uid)     //nolint:errcheck
}

// ── Meals ───────────────────────────────────────────────────────────────────

func todaysMeals(uid int64) ([]Meal, error) {
	rows, err := db.Query(`
		SELECT m.id, m.name, m.kcal, m.grams, m.prot, m.fat, m.carb,
		       strftime('%H:%M', m.eaten_at, '+3 hours') AS t,
		       EXISTS(SELECT 1 FROM favorite f WHERE f.user_id=m.user_id AND f.name=m.name) AS fav
		FROM meal m
		WHERE m.user_id=? AND m.day=`+mskDayExpr+`
		ORDER BY m.eaten_at DESC
		LIMIT 20`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	meals := []Meal{}
	for rows.Next() {
		var m Meal
		if err := rows.Scan(&m.ID, &m.Name, &m.Kcal, &m.Grams, &m.Prot, &m.Fat, &m.Carb, &m.Time, &m.Fav); err != nil {
			return nil, err
		}
		meals = append(meals, m)
	}
	return meals, rows.Err()
}

// donutTotals is the derived daily score — SUM over today's meals, never stored.
func donutTotals(uid int64) (kcal int, prot float64, err error) {
	err = db.QueryRow(`
		SELECT COALESCE(SUM(kcal),0), COALESCE(SUM(prot),0)
		FROM meal WHERE user_id=? AND day=`+mskDayExpr, uid).Scan(&kcal, &prot)
	return
}

func insertMeal(uid int64, in MealInput) (int64, error) {
	res, err := db.Exec(`
		INSERT INTO meal (user_id, day, name, kcal, grams, prot, fat, carb)
		VALUES (?, `+mskDayExpr+`, ?, ?, ?, ?, ?, ?)`,
		uid, in.Name, in.Kcal, in.Grams, in.Prot, in.Fat, in.Carb)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func updateMeal(uid, id int64, in MealInput) error {
	_, err := db.Exec(`
		UPDATE meal SET name=?, kcal=?, grams=?, prot=?, fat=?, carb=?
		WHERE id=? AND user_id=?`,
		in.Name, in.Kcal, in.Grams, in.Prot, in.Fat, in.Carb, id, uid)
	return err
}

func deleteMeal(uid, id int64) error {
	_, err := db.Exec(`DELETE FROM meal WHERE id=? AND user_id=?`, id, uid)
	return err
}

// ── Favorites (persist; never day-swept) ─────────────────────────────────────

func favorites(uid int64) ([]Favorite, error) {
	rows, err := db.Query(`
		SELECT id, name, kcal, grams, prot, fat, carb
		FROM favorite WHERE user_id=? ORDER BY name`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	favs := []Favorite{}
	for rows.Next() {
		var f Favorite
		if err := rows.Scan(&f.ID, &f.Name, &f.Kcal, &f.Grams, &f.Prot, &f.Fat, &f.Carb); err != nil {
			return nil, err
		}
		favs = append(favs, f)
	}
	return favs, rows.Err()
}

// upsertFavorite saves/updates a meal template by name (the star action).
func upsertFavorite(uid int64, in MealInput) error {
	_, err := db.Exec(`
		INSERT INTO favorite (user_id, name, kcal, grams, prot, fat, carb)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, name) DO UPDATE SET
			kcal=excluded.kcal, grams=excluded.grams,
			prot=excluded.prot, fat=excluded.fat, carb=excluded.carb`,
		uid, in.Name, in.Kcal, in.Grams, in.Prot, in.Fat, in.Carb)
	return err
}

func deleteFavorite(uid, id int64) error {
	_, err := db.Exec(`DELETE FROM favorite WHERE id=? AND user_id=?`, id, uid)
	return err
}

// ── AI usage + scan economy ──────────────────────────────────────────────────

func aiUsesToday(uid int64) int {
	var n int
	db.QueryRow(`SELECT COALESCE(uses,0) FROM ai_usage WHERE user_id=? AND day=`+mskDayExpr, uid).Scan(&n) //nolint:errcheck
	return n
}

func bumpAiUsage(uid int64) {
	db.Exec(`
		INSERT INTO ai_usage (user_id, day, uses) VALUES (?, `+mskDayExpr+`, 1)
		ON CONFLICT(user_id, day) DO UPDATE SET uses = uses + 1`, uid) //nolint:errcheck
}

func getStatus(uid int64) (Status, error) {
	var s Status
	err := db.QueryRow(`SELECT role, free_scans_left, daily_limit FROM users WHERE id=?`, uid).
		Scan(&s.Role, &s.FreeScansLeft, &s.DailyLimit)
	if err == sql.ErrNoRows {
		return Status{}, fmt.Errorf("user %d not found", uid)
	}
	return s, err
}

// canScan reports whether the user may run an AI op now, and a reason code if not.
//   pro    → always allowed
//   tester → allowed while today's uses < daily_limit  (else "daily_limit")
//   free   → allowed while free_scans_left > 0         (else "no_scans")
func canScan(s Status, usesToday int) (bool, string) {
	switch s.Role {
	case "pro":
		return true, ""
	case "tester":
		if usesToday < s.DailyLimit {
			return true, ""
		}
		return false, "daily_limit"
	default: // free
		if s.FreeScansLeft > 0 {
			return true, ""
		}
		return false, "no_scans"
	}
}

// consumeScan records one AI op: bump the daily counter, and for free users burn
// one of the lifetime free scans. Manual/favorite adds never call this.
func consumeScan(uid int64, s Status) {
	bumpAiUsage(uid)
	if s.Role == "free" {
		db.Exec(`UPDATE users SET free_scans_left = free_scans_left - 1
		         WHERE id=? AND free_scans_left > 0`, uid) //nolint:errcheck
	}
}

// statusLabel renders the Credits-screen "Статус" line.
func statusLabel(s Status) string {
	switch s.Role {
	case "pro":
		return "PRO · ∞"
	case "tester":
		return fmt.Sprintf("Тестер · %d/день", s.DailyLimit)
	default:
		return fmt.Sprintf("Бесплатно · %d осталось", s.FreeScansLeft)
	}
}
