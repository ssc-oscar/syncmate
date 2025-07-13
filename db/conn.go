package db

import (
	"fmt"
	"log"
	"os"
	"time"

	d1 "github.com/intmian/mian_go_lib/fork/d1_gorm_adapter"
	"github.com/intmian/mian_go_lib/fork/d1_gorm_adapter/gormd1"
	_ "github.com/intmian/mian_go_lib/fork/d1_gorm_adapter/stdlib"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type ConnectProps struct {
	apiToken   string
	accountId  string
	databaseId string
}

func ConnectDB(p ConnectProps) (*gorm.DB, error) {
	defaultDSN := fmt.Sprintf("d1://%s:%s@%s", p.accountId, p.apiToken, p.databaseId)
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
