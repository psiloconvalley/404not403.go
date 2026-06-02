package main

import (
	"database/sql"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"

	_ "github.com/lib/pq"
)

var db *sql.DB

func main() {
	// 1. Database Connection
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Println("ℹ️  No DATABASE_URL found. Running without DB.")
	} else {
		var err error
		db, err = sql.Open("postgres", dbURL)
		if err != nil {
			log.Printf("⚠️  DB Error: %v", err)
		} else {
			err = db.Ping()
			if err != nil {
				log.Printf("⚠️  DB Ping Failed: %v", err)
			} else {
				log.Println("✅ Postgres Connected.")
			}
		}
	}

	// 2. Port
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// 3. Static Files
	fs := http.FileServer(http.Dir("static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	// 4. Routes
	http.HandleFunc("/", handleHome)
	http.HandleFunc("/simulate/404", handleSimulate404)
	http.HandleFunc("/simulate/403", handleSimulate403)

	// 5. Start Server
	log.Printf("🚀 404not403 Engine Online. Port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	lp := filepath.Join("templates", "index.html")
	tmpl, err := template.ParseFiles(lp)
	if err != nil {
		log.Printf("Template Error: %v", err)
		http.Error(w, "System Error", 500)
		return
	}
	tmpl.Execute(w, nil)
}

func handleSimulate404(w http.ResponseWriter, r *http.Request) {
	go logEvent(404, "Not Found - resource is missing.")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte(`{
		"status": 404,
		"error": "Not Found",
		"message": "The resource is missing.",
		"tip": "It is gone. Not forbidden. Just gone."
	}`))
}
func handleSimulate403(w http.ResponseWriter, r *http.Request) {
	go logEvent(403, "Forbidden - access denied.")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	w.Write([]byte(`{
		"status": 403,
		"error": "Forbidden",
		"message": "Access denied.",
		"tip": "It exists. You just are not on the list."
	}`))
}
func logEvent(statusCode int, message string) {
	if db == nil {
		log.Println("⚠️  logEvent: db is nil, skipping insert.")
		return
	}
	_, err := db.Exec(
		"INSERT INTO logs (status_code, message) VALUES ($1, $2)",
		statusCode,
		message,
	)
	if err != nil {
		log.Printf("⚠️  logEvent insert error: %v", err)
	} else {
		log.Printf("✅ Logged: %d - %s", statusCode, message)
	}
}
