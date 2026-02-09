package rdb

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/idivarts/backend-sls/pkg/myutil"
	_ "github.com/lib/pq"
)

type KeySecretJson struct {
	RDB struct {
		User     string `json:"user"`
		Password string `json:"password"`
		Host     string `json:"host"`
		Port     int    `json:"port"`
		Database string `json:"database"`
	} `json:"rdb"`
}

var DB *sql.DB

func init() {
	basePath := "."
	if myutil.IsTest() {
		basePath = "/Users/rsinha/iDiv/backend-sls/"
	}
	path := filepath.Join(basePath, "key-secrets.json")
	file, err := os.Open(path)
	if err != nil {
		log.Printf("could not open key-secrets.json: %v", err)
		return
	}
	defer file.Close()

	var secrets KeySecretJson
	if err := json.NewDecoder(file).Decode(&secrets); err != nil {
		log.Printf("could not decode key-secrets.json: %v", err)
		return
	}
	// log.Println("Database", secrets.Database)

	dbUser := secrets.RDB.User
	dbPass := secrets.RDB.Password
	dbHost := secrets.RDB.Host
	dbPort := secrets.RDB.Port
	dbName := secrets.RDB.Database

	if dbUser == "" {
		dbUser = "root"
	}
	if dbPass == "" {
		dbPass = "password"
	}
	if dbHost == "" {
		dbHost = "localhost"
	}
	if dbPort == 0 {
		dbPort = 5432 // PostgreSQL default port
	}
	if dbName == "" {
		dbName = "mydatabase"
	}

	// Construct the PostgreSQL DSN (Data Source Name)
	// Format: host=<host> port=<port> user=<user> password=<password> dbname=<dbname> sslmode=disable
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s",
		dbHost, dbPort, dbUser, dbPass, dbName)

	// Open the database connection
	DB, err = sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("Failed to connect to Postgres: %v", err)
	}

	// Verify the connection
	if err = DB.Ping(); err != nil {
		log.Fatalf("Failed to ping Postgres: %v", err)
	}

	log.Println("Successfully connected to Postgres")
}
