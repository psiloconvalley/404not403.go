package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"

	_ "github.com/lib/pq" // Postgres driver
)

var db *sql.DB

func main() {
	// 1. Get DB URL from Railway Environment Variables
	dbURL := os.Getenv("DATABASE_URL")
	var err error
	db, err = sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal(err)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// ... (Rest of your handlers) ...

	log.Printf("404not403 System Online. Database Connected.")
	http.ListenAndServe(":"+port, nil)
}
