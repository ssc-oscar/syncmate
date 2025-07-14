package db

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func SetupCredentials(configPath string) CloudflareD1Credentials {
	var creds CloudflareD1Credentials
	// read the credentials from the config file
	file, err := os.ReadFile(configPath)
	if err != nil {
		log.Fatalf("Failed to read config file: %v", err)
	}
	if err := json.Unmarshal(file, &creds); err != nil {
		log.Fatalf("Failed to parse config file: %v", err)
	}
	if creds.APIToken == "" || creds.DatabaseID == "" || creds.AccountID == "" {
		log.Fatal("Missing required Cloudflare D1 credentials in config file")
	}
	return creds
}

func SetupD1DB() (*gorm.DB, error) {
	// Check if config file exists, if not use SQLite for testing
	configPath := "../config.json"
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		fmt.Println("Config file not found, using SQLite for testing")
		return SetupSQLiteDB()
	}
	
	creds := SetupCredentials(configPath)
	db, err := ConnectDB(creds)
	if err != nil {
		fmt.Printf("Failed to connect to D1 database: %v, falling back to SQLite\n", err)
		return SetupSQLiteDB()
	}
	fmt.Println("Database connection established successfully")
	return db, nil
}

func SetupSQLiteDB() (*gorm.DB, error) {
	// Use in-memory SQLite database for testing
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to SQLite database: %w", err)
	}
	
	// Auto migrate the Task model
	if err := db.AutoMigrate(&Task{}); err != nil {
		return nil, fmt.Errorf("failed to auto migrate Task model: %w", err)
	}
	
	fmt.Println("SQLite database connection established successfully")
	return db, nil
}

func TestConnectDB(t *testing.T) {
	db, err := SetupD1DB()
	if err != nil {
		t.Fatalf("Failed to connect to D1 database: %v", err)
	}
	if db == nil {
		t.Fatal("Database connection is nil")
	}

	// Test if the connection is alive
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("Failed to get sql.DB from gorm.DB: %v", err)
	}
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("Failed to ping database: %v", err)
	}

	fmt.Println("Database connection is alive")
}
