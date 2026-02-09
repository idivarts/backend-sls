package rdb

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	_ "github.com/go-sql-driver/mysql"
	"github.com/idivarts/backend-sls/pkg/myutil"
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
		dbPort = 3306
	}
	if dbName == "" {
		dbName = "mydatabase"
	}

	// Construct the DSN (Data Source Name)
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true",
		dbUser, dbPass, dbHost, dbPort, dbName)

	// Open the database connection
	DB, err = sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("Failed to connect to MySQL: %v", err)
	}

	// Verify the connection
	if err = DB.Ping(); err != nil {
		log.Fatalf("Failed to ping MySQL: %v", err)
	}

	log.Println("Successfully connected to MySQL")
}
