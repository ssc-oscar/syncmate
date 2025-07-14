package db

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"testing"
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

func TestConnectDB(t *testing.T) {
	creds := SetupCredentials("../config.json")
	db, err := ConnectDB(creds)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	if db == nil {
		t.Fatal("Database connection is nil")
	}
	fmt.Println("Database connection established successfully")
}
