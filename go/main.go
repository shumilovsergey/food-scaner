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
	approve   := flag.String("approve", "", "promote user to tester by auth_id")
	revoke    := flag.String("revoke", "", "demote user to free by auth_id")
	setLimit  := flag.String("set-limit", "", "set daily scan limit (for testers): <auth_id> <limit>")
	setRole   := flag.String("set-role", "", "set role (free|tester|pro): <auth_id> <role>")
	setPro    := flag.String("set-pro", "", "set PRO expiry date YYYY-MM-DD: <auth_id> <date>")
	addScans  := flag.String("add-scans", "", "add owned scans: <auth_id> <n>")
	listUsers := flag.Bool("list-users", false, "list all users")
	flag.Parse()

	if *approve != "" || *revoke != "" || *setLimit != "" || *setRole != "" ||
		*setPro != "" || *addScans != "" || *listUsers {
		initDB()
		runAdmin(*approve, *revoke, *setLimit, *setRole, *setPro, *addScans, *listUsers)
		return
	}

	// ── Server ────────────────────────────────────────────────────────────
	log.SetFlags(log.Ldate | log.Ltime | log.LUTC)
	initDB()

	mux := http.NewServeMux()

	mux.HandleFunc("GET /login", handleLogin)
	mux.HandleFunc("GET /logout", handleLogout)
	mux.HandleFunc("GET /api/me", handleMe)
	mux.HandleFunc("POST /api/scan", requireAuth(handleScan))

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

func runAdmin(approve, revoke, setLimit, setRole, setPro, addScans string, listUsersFlag bool) {
	if listUsersFlag {
		users, err := listUsers()
		if err != nil {
			log.Fatalf("list users: %v", err)
		}
		fmt.Printf("%-5s %-20s %-8s %-26s %s\n", "ID", "AUTH_ID", "METHOD", "STATUS", "USERNAME")
		fmt.Println("--------------------------------------------------------------------------")
		for _, u := range users {
			fmt.Printf("%-5d %-20s %-8s %-26s %s\n",
				u.ID, u.AuthID, u.Method, userStatusStr(u), u.Username)
		}
		return
	}

	// --approve → promote to tester
	if approve != "" {
		res, err := db.Exec(`UPDATE users SET role='tester' WHERE auth_id=?`, approve)
		if err != nil {
			log.Fatalf("approve: %v", err)
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			fmt.Printf("user %q not found\n", approve)
		} else {
			fmt.Printf("promoted to tester: %s\n", approve)
		}
	}

	// --revoke → demote to free
	if revoke != "" {
		res, err := db.Exec(`UPDATE users SET role='free' WHERE auth_id=?`, revoke)
		if err != nil {
			log.Fatalf("revoke: %v", err)
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			fmt.Printf("user %q not found\n", revoke)
		} else {
			fmt.Printf("demoted to free: %s\n", revoke)
		}
	}

	// --set-limit <auth_id> <limit>
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

	// --set-role <auth_id> <role>
	if setRole != "" {
		args := flag.Args()
		if len(args) < 1 {
			log.Fatal("usage: --set-role <auth_id> <role>")
		}
		role := args[0]
		if role != "free" && role != "tester" && role != "pro" {
			log.Fatalf("invalid role %q (must be free|tester|pro)", role)
		}
		res, err := db.Exec(`UPDATE users SET role=? WHERE auth_id=?`, role, setRole)
		if err != nil {
			log.Fatalf("set-role: %v", err)
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			fmt.Printf("user %q not found\n", setRole)
		} else {
			fmt.Printf("set role to %q for: %s\n", role, setRole)
		}
	}

	// --set-pro <auth_id> <YYYY-MM-DD>
	if setPro != "" {
		args := flag.Args()
		if len(args) < 1 {
			log.Fatal("usage: --set-pro <auth_id> <YYYY-MM-DD>")
		}
		date := args[0]
		res, err := db.Exec(`UPDATE users SET role='pro', pro_until=? WHERE auth_id=?`, date, setPro)
		if err != nil {
			log.Fatalf("set-pro: %v", err)
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			fmt.Printf("user %q not found\n", setPro)
		} else {
			fmt.Printf("set PRO until %s for: %s\n", date, setPro)
		}
	}

	// --add-scans <auth_id> <n>
	if addScans != "" {
		args := flag.Args()
		if len(args) < 1 {
			log.Fatal("usage: --add-scans <auth_id> <n>")
		}
		n, err := strconv.Atoi(args[0])
		if err != nil || n <= 0 {
			log.Fatalf("invalid scan count %q", args[0])
		}
		res, err := db.Exec(`UPDATE users SET owned_scans = owned_scans + ? WHERE auth_id=?`, n, addScans)
		if err != nil {
			log.Fatalf("add-scans: %v", err)
		}
		rows, _ := res.RowsAffected()
		if rows == 0 {
			fmt.Printf("user %q not found\n", addScans)
		} else {
			fmt.Printf("added %d scans for: %s\n", n, addScans)
		}
	}
}
