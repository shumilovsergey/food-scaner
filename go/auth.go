package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const ctxUserID contextKey = "userID"

// ── JWT ──────────────────────────────────────────────────────────────────

type Claims struct {
	UserID int64 `json:"user_id"`
	jwt.RegisteredClaims
}

func secretKey() []byte { return []byte(os.Getenv("SECRET_KEY")) }

func makeToken(userID int64) (string, error) {
	claims := Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(30 * 24 * time.Hour)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secretKey())
}

func parseToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return secretKey(), nil
	})
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	return token.Claims.(*Claims), nil
}

// ── Cookie ───────────────────────────────────────────────────────────────

func setCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   30 * 24 * 3600,
	})
}

func clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: "session", Value: "", Path: "/", MaxAge: -1})
}

func userIDFromCtx(r *http.Request) int64 {
	return r.Context().Value(ctxUserID).(int64)
}

// ── Middlewares ───────────────────────────────────────────────────────────

func requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session")
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		claims, err := parseToken(cookie.Value)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), ctxUserID, claims.UserID)
		next(w, r.WithContext(ctx))
	}
}


// ── Auth handlers ─────────────────────────────────────────────────────────

func handleLogin(w http.ResponseWriter, r *http.Request) {
	authURL := os.Getenv("AUTH_URL")
	appURL := os.Getenv("APP_URL")
	http.Redirect(w, r, authURL+"/?redirect="+appURL+"/", http.StatusFound)
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	clearCookie(w)
	http.Redirect(w, r, "/", http.StatusFound)
}

// handleCallback is called when the user lands on / with ?code=...
func handleCallback(w http.ResponseWriter, r *http.Request) bool {
	code := r.URL.Query().Get("code")
	if code == "" {
		return false
	}
	log.Printf("callback: got code=%s", code)

	authInternal := os.Getenv("AUTH_INTERNAL")
	appToken := os.Getenv("APP_TOKEN")

	body, _ := json.Marshal(map[string]string{
		"code":      code,
		"app_token": appToken,
	})
	resp, err := http.Post(authInternal+"/exchange", "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("exchange error: %v", err)
		http.Error(w, "auth exchange failed", http.StatusBadGateway)
		return true
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	log.Printf("exchange status=%d body=%s", resp.StatusCode, string(raw))
	if resp.StatusCode != http.StatusOK {
		http.Error(w, "auth exchange error: "+string(raw), http.StatusUnauthorized)
		return true
	}

	var result struct {
		Ok     bool           `json:"ok"`
		Method string         `json:"method"`
		User   map[string]any `json:"user"`
	}
	if err := json.Unmarshal(raw, &result); err != nil || !result.Ok {
		http.Error(w, "invalid auth response", http.StatusUnauthorized)
		return true
	}

	authID := extractID(result.User)
	username := extractUsername(result.User)

	user, err := upsertUser(authID, result.Method, username)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return true
	}

	token, err := makeToken(user.ID)
	if err != nil {
		http.Error(w, "token error", http.StatusInternalServerError)
		return true
	}
	setCookie(w, token)
	http.Redirect(w, r, "/", http.StatusSeeOther)
	return true
}

// handleMe returns current user info as JSON (used by the frontend).
func handleMe(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session")
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	claims, err := parseToken(cookie.Value)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	u, err := getUserByID(claims.UserID)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	todayScans, _ := countTodayScans(u.ID)
	totalScans, _ := countTotalScans(u.ID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"id":              u.ID,
		"auth_id":         u.AuthID,
		"username":        u.Username,
		"role":            u.Role,
		"daily_limit":     u.DailyLimit,
		"free_scans_left": u.FreeScansLeft,
		"owned_scans":     u.OwnedScans,
		"pro_until":       proUntilStr(u),
		"today_scans":     todayScans,
		"total_scans":     totalScans,
	})
}

// ── Helpers ───────────────────────────────────────────────────────────────

func extractID(user map[string]any) string {
	switch v := user["id"].(type) {
	case float64:
		return fmt.Sprintf("%.0f", v)
	case string:
		return v
	}
	return ""
}

func extractUsername(user map[string]any) string {
	parts := []string{}
	for _, k := range []string{"first_name", "last_name"} {
		if s, _ := user[k].(string); s != "" {
			parts = append(parts, s)
		}
	}
	if name := strings.Join(parts, " "); name != "" {
		return name
	}
	if u, _ := user["username"].(string); u != "" {
		return u
	}
	if e, _ := user["email"].(string); e != "" {
		return strings.SplitN(e, "@", 2)[0]
	}
	return "user"
}
