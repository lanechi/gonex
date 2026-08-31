package db

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lanechi/gonex/config"
	"github.com/lanechi/gonex/contrib/gormlog"
	"github.com/lanechi/gonex/logging"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	postgresDB syncPostgresDB
)

type syncPostgresDB struct {
	mu sync.RWMutex
	db *gorm.DB
}

type postgresSettings struct {
	DSN      string
	URL      string
	Host     string
	Port     int
	Username string
	User     string
	Password string
	Database string
	Name     string
	SSLMode  string
	TimeZone string
}

// InitializePostgres opens the configured PostgreSQL database with the
// framework logger. Settings are read from DATABASE_* values in .env or the
// process environment; config.yaml is intentionally not used for database
// settings.
func InitializePostgres(configuration config.Config) error {
	if configuration == nil {
		return fmt.Errorf("postgres configuration is nil")
	}
	configurationValues := loadPostgresSettings(configuration)
	level := parsePostgresLogLevel(configuration.GetString("DATABASE_LOG_LEVEL"))
	slowThreshold := 200 * time.Millisecond
	if configured := configuration.GetString("DATABASE_LOG_SLOW_THRESHOLD"); configured != "" {
		parsed, err := time.ParseDuration(configured)
		if err != nil {
			return fmt.Errorf("invalid DATABASE_LOG_SLOW_THRESHOLD: %w", err)
		}
		slowThreshold = parsed
	}
	dialector, err := postgresDialector(configurationValues)
	if err != nil {
		return err
	}
	postgresDB.mu.Lock()
	defer postgresDB.mu.Unlock()
	if postgresDB.db != nil {
		return fmt.Errorf("postgres database is already initialized")
	}
	opened, err := gorm.Open(dialector, &gorm.Config{
		Logger: gormlog.New(
			logging.Default(),
			gormlog.WithLogLevel(level),
			gormlog.WithSlowThreshold(slowThreshold),
		),
	})
	if err != nil {
		return fmt.Errorf("open postgres database: %w", err)
	}
	postgresDB.db = opened
	return nil
}

func loadPostgresSettings(configuration config.Config) postgresSettings {
	result := postgresSettings{
		DSN:      strings.TrimSpace(configuration.GetString("DATABASE_DSN")),
		URL:      strings.TrimSpace(configuration.GetString("DATABASE_URL")),
		Host:     strings.TrimSpace(configuration.GetString("DATABASE_HOST")),
		Port:     configuration.GetInt("DATABASE_PORT"),
		Username: strings.TrimSpace(configuration.GetString("DATABASE_USERNAME")),
		User:     strings.TrimSpace(configuration.GetString("DATABASE_USER")),
		Password: configuration.GetString("DATABASE_PASSWORD"),
		Database: strings.TrimSpace(configuration.GetString("DATABASE_DATABASE")),
		Name:     strings.TrimSpace(configuration.GetString("DATABASE_NAME")),
		SSLMode:  strings.TrimSpace(configuration.GetString("DATABASE_SSLMODE")),
		TimeZone: strings.TrimSpace(configuration.GetString("DATABASE_TIMEZONE")),
	}
	return result
}

// Postgres returns the initialized PostgreSQL database.
func Postgres() *gorm.DB {
	postgresDB.mu.RLock()
	defer postgresDB.mu.RUnlock()
	if postgresDB.db == nil {
		panic("postgres database is not initialized")
	}
	return postgresDB.db
}

// ClosePostgres releases the PostgreSQL connection during shutdown.
func ClosePostgres() error {
	postgresDB.mu.Lock()
	defer postgresDB.mu.Unlock()
	if postgresDB.db == nil {
		return nil
	}
	sqlDatabase, err := postgresDB.db.DB()
	if err != nil {
		return err
	}
	if err := sqlDatabase.Close(); err != nil {
		return err
	}
	postgresDB.db = nil
	return nil
}

func postgresDialector(configuration postgresSettings) (gorm.Dialector, error) {
	dsn := strings.TrimSpace(configuration.DSN)
	if dsn == "" {
		dsn = strings.TrimSpace(configuration.URL)
	}
	if dsn == "" {
		dsn = postgresDSN(configuration)
	}
	if dsn == "" {
		return nil, fmt.Errorf("DATABASE_DSN, DATABASE_URL, or PostgreSQL connection fields are required")
	}
	return postgres.Open(dsn), nil
}

func postgresDSN(configuration postgresSettings) string {
	user := configuration.Username
	if user == "" {
		user = configuration.User
	}
	database := configuration.Database
	if database == "" {
		database = configuration.Name
	}
	if user == "" || configuration.Host == "" || database == "" {
		return ""
	}
	port := configuration.Port
	if port == 0 {
		port = 5432
	}
	sslMode := configuration.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}
	timeZone := configuration.TimeZone
	if timeZone == "" {
		timeZone = "UTC"
	}
	connection := &url.URL{
		Scheme: "postgres",
		Host:   net.JoinHostPort(strings.Trim(configuration.Host, "[]"), strconv.Itoa(port)),
		Path:   "/" + database,
	}
	if configuration.Password == "" {
		connection.User = url.User(user)
	} else {
		connection.User = url.UserPassword(user, configuration.Password)
	}
	query := connection.Query()
	query.Set("sslmode", sslMode)
	query.Set("TimeZone", timeZone)
	connection.RawQuery = query.Encode()
	return connection.String()
}

func parsePostgresLogLevel(value string) logger.LogLevel {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "silent":
		return logger.Silent
	case "error":
		return logger.Error
	case "info":
		return logger.Info
	default:
		return logger.Warn
	}
}
