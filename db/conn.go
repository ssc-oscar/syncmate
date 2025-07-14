package db

import (
	"fmt"
	"log"
	"os"
	"time"

	d1 "github.com/hrz6976/syncmate/d1_gorm_adapter"
	"github.com/hrz6976/syncmate/d1_gorm_adapter/gormd1"
	_ "github.com/hrz6976/syncmate/d1_gorm_adapter/stdlib"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type CloudflareD1Credentials struct {
	APIToken   string `json:"api_token"`
	DatabaseID string `json:"database_id"`
	AccountID  string `json:"account_id"`
}

func ConnectDB(p CloudflareD1Credentials) (*gorm.DB, error) {
	defaultDSN := fmt.Sprintf("d1://%s:%s@%s", p.AccountID, p.APIToken, p.DatabaseID)
	d1.TraceOn(os.Stdout)
	newLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags), // io writer
		logger.Config{
			SlowThreshold:        time.Second, // Slow SQL threshold
			LogLevel:             logger.Info, // Log level
			ParameterizedQueries: true,        // Don't include params in the SQL log
			Colorful:             true,        // Disable color
		},
	)

	gdb, err := gorm.Open(gormd1.Open(defaultDSN), &gorm.Config{
		SkipDefaultTransaction:                   true,
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger:                                   newLogger,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	return gdb, nil
}

func CloseDB(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get sql.DB from gorm.DB: %w", err)
	}
	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("failed to close database connection: %w", err)
	}
	return nil
}
