package cmd

import (
	"github.com/hrz6976/syncmate/db"
)

type CloudflareCredentials struct {
	// Explicitly define the fields to avoid duplicate json tags
	AccountID  string `json:"account_id"`
	ApiToken   string `json:"api_token,omitempty"`
	AccessKey  string `json:"access_key,omitempty"`
	SecretKey  string `json:"secret_key,omitempty"`
	Bucket     string `json:"bucket,omitempty"`
	DatabaseID string `json:"database_id,omitempty"`
}

var dbHandle *db.DB
var config *CloudflareCredentials

func connectDB() (*db.DB, error) {
	if dbHandle != nil {
		return dbHandle, nil
	}
	cloudflareD1Creds := db.CloudflareD1Credentials{
		APIToken:   config.ApiToken,
		DatabaseID: config.DatabaseID,
		AccountID:  config.AccountID,
	}
	gormDB, err := db.ConnectDB(cloudflareD1Creds)
	if err != nil {
		return nil, err
	}
	dbHandle = db.NewDB(gormDB)
	return dbHandle, nil
}
