package logger_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/hrz6976/syncmate/logger"
)

func TestLogger(t *testing.T) {
	// Test the logger
	logger.Debug("这是一条 DEBUG 消息", "key", "value", "number", 42)
	logger.Info("这是一条 INFO 消息", "user", "alice", "action", "login")
	logger.Warn("这是一条 WARN 消息", "warning", "高内存使用", "memory", "85%")
	logger.Error("这是一条 ERROR 消息", "error", "连接失败", "retry", 3)

	// Test with context
	ctx := context.Background()
	logger.InfoContext(ctx, "带上下文的消息", "requestId", "abc123")

	// Test with additional attributes
	ctxlogger := logger.With("component", "database")
	ctxlogger.Info("数据库连接成功", "host", "localhost", "port", 5432)

	// Test with group
	dbLogger := logger.WithGroup("db")
	dbLogger.Error("查询失败", "query", "SELECT * FROM users", "duration", "5.2s")

	// Test different levels
	logger.InitLoggerWithLevel(slog.LevelWarn)
	logger.Debug("这条 DEBUG 消息不会显示")
	logger.Info("这条 INFO 消息不会显示")
	logger.Warn("这条 WARN 消息会显示")
	logger.Error("这条 ERROR 消息会显示")
}
