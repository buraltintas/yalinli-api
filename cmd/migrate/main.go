package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
	"github.com/pressly/goose/v3"
)

func main() {
	_ = godotenv.Load(".env")
	databaseURL := strings.TrimSpace(os.Getenv("MIGRATIONS_DATABASE_URL"))
	if databaseURL == "" {
		databaseURL = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	}
	if databaseURL == "" {
		log.Fatal("DATABASE_URL or MIGRATIONS_DATABASE_URL must be set")
	}
	if len(os.Args) < 2 {
		log.Fatal("usage: go run ./cmd/migrate [up|down]")
	}
	cmd := os.Args[1]
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err := goose.SetDialect("postgres"); err != nil {
		log.Fatal(err)
	}
	if cmd == "up" {
		err = goose.Up(db, "migrations")
	} else if cmd == "down" {
		err = goose.Down(db, "migrations")
	} else {
		log.Fatalf("unsupported command: %s", cmd)
	}
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("ok")
}
