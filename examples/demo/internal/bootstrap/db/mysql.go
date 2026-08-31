package db

// MySQL support is intentionally disabled in the canonical demo. To enable
// it, add gorm.io/driver/mysql to the module and uncomment the implementation
// below. Keep its names database-specific if PostgreSQL is also enabled.

/*
import (
	"fmt"
	"strings"
	"sync"

	"github.com/lanechi/gonex/config"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var mysqlDB struct {
	sync.RWMutex
	db *gorm.DB
}

type mysqlSettings struct {
	DSN      string
	Host     string
	Port     int
	User     string
	Password string
	Database string
}

func InitializeMySQL(configuration config.Config) error {
	if configuration == nil {
		return fmt.Errorf("mysql configuration is nil")
	}
	settings := mysqlSettings{
		DSN: configuration.GetString("DATABASE_DSN"), Host: configuration.GetString("DATABASE_HOST"),
		Port: configuration.GetInt("DATABASE_PORT"), User: configuration.GetString("DATABASE_USER"),
		Password: configuration.GetString("DATABASE_PASSWORD"), Database: configuration.GetString("DATABASE_NAME"),
	}
	dsn := strings.TrimSpace(settings.DSN)
	if dsn == "" {
		if settings.Port == 0 { settings.Port = 3306 }
		dsn = fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
			settings.User, settings.Password, settings.Host, settings.Port, settings.Database)
	}
	database, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil { return fmt.Errorf("open mysql database: %w", err) }
	mysqlDB.Lock()
	defer mysqlDB.Unlock()
	if mysqlDB.db != nil { return fmt.Errorf("mysql database is already initialized") }
	mysqlDB.db = database
	return nil
}

func MySQL() *gorm.DB {
	mysqlDB.RLock(); defer mysqlDB.RUnlock()
	if mysqlDB.db == nil { panic("mysql database is not initialized") }
	return mysqlDB.db
}

func CloseMySQL() error {
	mysqlDB.Lock(); defer mysqlDB.Unlock()
	if mysqlDB.db == nil { return nil }
	sqlDB, err := mysqlDB.db.DB()
	if err != nil { return err }
	if err := sqlDB.Close(); err != nil { return err }
	mysqlDB.db = nil
	return nil
}
*/
