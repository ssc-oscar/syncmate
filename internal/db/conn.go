package db

import (
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	d1 "github.com/intmian/mian_go_lib/fork/d1_gorm_adapter"
	"github.com/intmian/mian_go_lib/fork/d1_gorm_adapter/gormd1"
	_ "github.com/intmian/mian_go_lib/fork/d1_gorm_adapter/stdlib"
	"github.com/joho/godotenv"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestMain(m *testing.M) {
	var err = godotenv.Load("../dev.env")
	if err != nil {
		panic(err)
	}
	apiToken := os.Getenv("API_TOKEN")
	accountId := os.Getenv("ACCOUNT_ID")
	datebaseId := os.Getenv("DATABASE_ID")

	defaultDSN := fmt.Sprintf("d1://%s:%s@%s", accountId, apiToken, datebaseId)

	d1.TraceOn(os.Stdout)

	newLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags), // io writer
		logger.Config{
			SlowThreshold: time.Second, // Slow SQL threshold
			LogLevel:      logger.Info, // Log level
			// IgnoreRecordNotFoundError: true,        // Ignore ErrRecordNotFound error for logger
			ParameterizedQueries: true, // Don't include params in the SQL log
			Colorful:             true, // Disable color
		},
	)

	gdb, err := gorm.Open(gormd1.Open(defaultDSN), &gorm.Config{
		SkipDefaultTransaction:                   true,
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger:                                   newLogger,
	})
	if err != nil {
		panic(err)
	}

	return gdb
}
