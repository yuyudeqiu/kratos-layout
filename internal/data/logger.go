package data

import (
	"context"
	"errors"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"
)

// GormLogger is a custom GORM logger that forwards logs to Kratos logger.
type GormLogger struct {
	log           *log.Helper
	logLevel      glogger.LogLevel
	slowThreshold time.Duration
}

// NewGormLogger creates a new GormLogger.
func NewGormLogger(logger log.Logger, slowThreshold time.Duration) *GormLogger {
	return &GormLogger{
		log:           log.NewHelper(logger),
		logLevel:      glogger.Info, // Default level, can be overridden by LogMode
		slowThreshold: slowThreshold,
	}
}

// LogMode sets the log level.
func (l *GormLogger) LogMode(level glogger.LogLevel) glogger.Interface {
	newLogger := *l
	newLogger.logLevel = level
	return &newLogger
}

// Info logs info messages.
func (l *GormLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	if l.logLevel >= glogger.Info {
		l.log.WithContext(ctx).Infof(msg, data...)
	}
}

// Warn logs warning messages.
func (l *GormLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	if l.logLevel >= glogger.Warn {
		l.log.WithContext(ctx).Warnf(msg, data...)
	}
}

// Error logs error messages.
func (l *GormLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	if l.logLevel >= glogger.Error {
		l.log.WithContext(ctx).Errorf(msg, data...)
	}
}

// Trace logs SQL statements, slow queries, and database errors.
func (l *GormLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	if l.logLevel <= glogger.Silent {
		return
	}

	elapsed := time.Since(begin)
	switch {
	case err != nil && l.logLevel >= glogger.Error && !errors.Is(err, gorm.ErrRecordNotFound):
		sql, rows := fc()
		l.log.WithContext(ctx).Errorf("[GORM] SQL error: %v | elapsed: %v | rows: %d | sql: %s", err, elapsed, rows, sql)
	case elapsed > l.slowThreshold && l.slowThreshold > 0 && l.logLevel >= glogger.Warn:
		sql, rows := fc()
		l.log.WithContext(ctx).Warnf("[GORM] SLOW SQL >= %v | elapsed: %v | rows: %d | sql: %s", l.slowThreshold, elapsed, rows, sql)
	case l.logLevel >= glogger.Info:
		sql, rows := fc()
		l.log.WithContext(ctx).Infof("[GORM] elapsed: %v | rows: %d | sql: %s", elapsed, rows, sql)
	}
}
