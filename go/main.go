package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strconv"
)

//go:embed static
var staticFiles embed.FS

func main() {
	// ── CLI admin commands ────────────────────────────────────────────────
	approve := flag.String("approve", "", "approve user by auth_id")
	revoke := flag.String("revoke", "", "revoke user approval by auth_id")
	setLimit := flag.String("set-limit", "", "set daily scan limit: <auth_id> <limit>")
	listUsers := flag.Bool("list-users", false, "list all users")
	flag.Parse()

	if *approve != "" || *revoke != "" || *setLimit != "" || *listUsers {
		initDB()
		runAdmin(*approve, *revoke, *setLimit, *listUsers)
		return
	}

	// ── Server ────────────────────────────────────────────────────────────
	log.SetFlags(log.Ldate | log.Ltime | log.LUTC)
	initDB()

	mux := http.NewServeMux()

	mux.HandleFunc("GET /login", handleLogin)
	mux.HandleFunc("GET /logout", handleLogout)
	mux.HandleFunc("GET /api/me", handleMe)
	mux.HandleFunc("POST /api/scan", requireApproved(withDailyLimit(handleScan)))

	staticFS, _ := fs.Sub(staticFiles, "static")
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("GET / query: %s", r.URL.RawQuery)
		if handleCallback(w, r) {
			return
		}
		http.FileServer(http.FS(staticFS)).ServeHTTP(w, r)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("food-scaner listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func runAdmin(approve, revoke, setLimit string, listUsersFlag bool) {
	if listUsersFlag {
		users, err := listUsers()
		if err != nil {
			log.Fatalf("list users: %v", err)
		}
		fmt.Printf("%-5s %-20s %-10s %-8s %-10s %s\n", "ID", "AUTH_ID", "METHOD", "APPROVED", "LIMIT", "USERNAME")
		fmt.Println("-------------------------------------------------------------------")
		for _, u := range users {
			approved := "no"
			if u.Approved {
				approved = "YES"
			}
			fmt.Printf("%-5d %-20s %-10s %-8s %-10d %s\n",
				u.ID, u.AuthID, u.Method, approved, u.DailyLimit, u.Username)
		}
		return
	}

	if approve != "" {
		res, err := db.Exec(`UPDATE users SET approved=1 WHERE auth_id=?`, approve)
		if err != nil {
			log.Fatalf("approve: %v", err)
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			fmt.Printf("user %q not found\n", approve)
		} else {
			fmt.Printf("approved: %s\n", approve)
		}
	}

	if revoke != "" {
		res, err := db.Exec(`UPDATE users SET approved=0 WHERE auth_id=?`, revoke)
		if err != nil {
			log.Fatalf("revoke: %v", err)
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			fmt.Printf("user %q not found\n", revoke)
		} else {
			fmt.Printf("revoked: %s\n", revoke)
		}
	}

	if setLimit != "" {
		args := flag.Args()
		if len(args) < 1 {
			log.Fatal("usage: --set-limit <auth_id> <limit>")
		}
		limit, err := strconv.Atoi(args[0])
		if err != nil || limit < 0 {
			log.Fatalf("invalid limit %q", args[0])
		}
		res, err := db.Exec(`UPDATE users SET daily_limit=? WHERE auth_id=?`, limit, setLimit)
		if err != nil {
			log.Fatalf("set-limit: %v", err)
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			fmt.Printf("user %q not found\n", setLimit)
		} else {
			fmt.Printf("set daily limit to %d for: %s\n", limit, setLimit)
		}
	}
}
