package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/carloberd/db-reader/postgresql"
	"github.com/joho/godotenv"

	_ "github.com/lib/pq"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading env: %v", err)
	}

	// Parametri di connessione
	dbUser := os.Getenv("DB_USER")
	dbPass := os.Getenv("DB_PASS")
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbName := os.Getenv("DB_NAME")

	if dbUser == "" || dbHost == "" || dbName == "" {
		log.Fatal("Missing env variables DB_USER, DB_HOST or DB_NAME")
	}

	if dbPort == "" {
		dbPort = "5432" // Default PostgreSQL port
	}

	// Connetti al database
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPass, dbName)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("An error occurred connecting to database: %v", err)
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		log.Fatalf("An error occurred pinging the database: %v", err)
	}

	schema := "public" // Default Schema
	if len(os.Args) > 1 {
		schema = os.Args[1]
	}

	fmt.Printf("Connected to %s, schema: %s\n\n", dbName, schema)

	tables, err := postgresql.GetTableList(db, schema)
	if err != nil {
		log.Fatalf("An error occurred obtaining tables list: %v", err)
	}

	fmt.Printf("Availabe tables in %s:\n", schema)
	for i, tableName := range tables {
		fmt.Printf("%d. %s\n", i+1, tableName)
	}

	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("\nEnter table name (or 'q' to quit application): ")
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Printf("An error occurred reading input: %v\n", err)
			continue
		}

		input = strings.TrimSpace(input)

		if input == "q" || input == "exit" || input == "quit" {
			fmt.Println("Closing application.")
			break
		}

		table, err := postgresql.GetTableStructure(db, schema, input)
		if err != nil {
			fmt.Printf("Error fetching table structure: %v\n", err)
			continue
		}

		postgresql.PrintTableStructure(table)
	}
}
