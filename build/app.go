package main

// app.go — food-scaner HTTP handlers. All routes here are registered behind
// requireAuth in main.go, so sessionUserID(r) is always non-zero.

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
)

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// pathID pulls a {id} path value as int64.
func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

// ── GET /api/state — single SPA bootstrap ────────────────────────────────────

func handleState(w http.ResponseWriter, r *http.Request) {
	uid := sessionUserID(r)
	sweepOldDays(uid) // lazy MSK-day rollover happens here

	user, err := getUserByID(uid)
	if err != nil || user == nil {
		writeErr(w, http.StatusUnauthorized, "user not found")
		return
	}
	status, err := getStatus(uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	meals, err := todaysMeals(uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	favs, err := favorites(uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	kcal, prot, err := donutTotals(uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	uses := aiUsesToday(uid)

	writeJSON(w, http.StatusOK, map[string]any{
		"user": map[string]any{
			"name":   user.Name,
			"method": user.Method,
			"authId": user.AuthID,
		},
		"status":    statusLabel(status),
		"role":      status.Role,
		"usesToday": uses,
		"donut":     map[string]any{"kcal": kcal, "prot": prot},
		"meals":     meals,
		"favorites": favs,
	})
}

// ── Meals ────────────────────────────────────────────────────────────────────

func decodeMeal(r *http.Request) (MealInput, bool) {
	var in MealInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		return in, false
	}
	if in.Name == "" {
		in.Name = "Meal"
	}
	return in, true
}

// POST /api/meal — add a meal (manual / from favorite / confirmed AI result).
func handleMealAdd(w http.ResponseWriter, r *http.Request) {
	uid := sessionUserID(r)
	in, ok := decodeMeal(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}
	id, err := insertMeal(uid, in)
	if err != nil {
		log.Printf("meal uid=%d action=add error=%q", uid, err)
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	log.Printf("meal uid=%d id=%d action=add", uid, id)
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}

// POST /api/meal/{id} — edit an existing meal.
func handleMealUpdate(w http.ResponseWriter, r *http.Request) {
	uid := sessionUserID(r)
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	in, ok := decodeMeal(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}
	if err := updateMeal(uid, id, in); err != nil {
		log.Printf("meal uid=%d id=%d action=edit error=%q", uid, id, err)
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	log.Printf("meal uid=%d id=%d action=edit", uid, id)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// DELETE /api/meal/{id} — remove a meal.
func handleMealDelete(w http.ResponseWriter, r *http.Request) {
	uid := sessionUserID(r)
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	if err := deleteMeal(uid, id); err != nil {
		log.Printf("meal uid=%d id=%d action=delete error=%q", uid, id, err)
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	log.Printf("meal uid=%d id=%d action=delete", uid, id)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ── Favorites ────────────────────────────────────────────────────────────────

// POST /api/favorite — star a meal (upsert the template by name).
func handleFavoriteAdd(w http.ResponseWriter, r *http.Request) {
	uid := sessionUserID(r)
	in, ok := decodeMeal(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}
	if err := upsertFavorite(uid, in); err != nil {
		log.Printf("favorite uid=%d action=add error=%q", uid, err)
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	log.Printf("favorite uid=%d action=add name=%q", uid, in.Name)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// DELETE /api/favorite/{id} — remove a favorite from the library.
func handleFavoriteDelete(w http.ResponseWriter, r *http.Request) {
	uid := sessionUserID(r)
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	if err := deleteFavorite(uid, id); err != nil {
		log.Printf("favorite uid=%d id=%d action=delete error=%q", uid, id, err)
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	log.Printf("favorite uid=%d id=%d action=delete", uid, id)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ── AI scan ──────────────────────────────────────────────────────────────────

type scanReq struct {
	Mode      string `json:"mode"`       // "photo" | "text"
	Image     string `json:"image"`      // base64 (photo)
	MediaType string `json:"media_type"` // photo
	Text      string `json:"text"`       // text
}

// POST /api/scan — run an AI nutrition estimate. Enforces the daily/lifetime quota
// (this is the "AI submit" that counts), but does NOT persist: the UI prefills the
// meal sheet from the result and the user saves via POST /api/meal.
func handleScan(w http.ResponseWriter, r *http.Request) {
	uid := sessionUserID(r)
	var req scanReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}

	status, err := getStatus(uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if ok, reason := canScan(status, aiUsesToday(uid)); !ok {
		if reason == "daily_limit" {
			log.Printf("scan uid=%d mode=%s result=blocked reason=daily_limit", uid, req.Mode)
			writeErr(w, http.StatusTooManyRequests, "Дневной лимит сканов достигнут")
		} else {
			log.Printf("scan uid=%d mode=%s result=blocked reason=no_scans", uid, req.Mode)
			writeErr(w, http.StatusForbidden, "Бесплатные сканы закончились")
		}
		return
	}

	var content []map[string]any
	switch req.Mode {
	case "photo":
		if req.Image == "" {
			writeErr(w, http.StatusBadRequest, "no image")
			return
		}
		content = photoContent(req.Image, req.MediaType)
	case "text":
		if req.Text == "" {
			writeErr(w, http.StatusBadRequest, "no text")
			return
		}
		content = textContent(req.Text)
	default:
		writeErr(w, http.StatusBadRequest, "bad mode")
		return
	}

	result, err := scanWithClaude(content)
	if err != nil {
		if errors.Is(err, errNoCredits) {
			log.Printf("scan uid=%d mode=%s result=error reason=no_credits", uid, req.Mode)
			writeErr(w, http.StatusPaymentRequired, "Сервис временно недоступен")
			return
		}
		log.Printf("scan uid=%d mode=%s result=error error=%q", uid, req.Mode, err)
		writeErr(w, http.StatusInternalServerError, "scan failed")
		return
	}

	consumeScan(uid, status) // counts only on a successful AI op
	log.Printf("scan uid=%d mode=%s result=ok name=%q", uid, req.Mode, result.Name)
	writeJSON(w, http.StatusOK, result)
}
