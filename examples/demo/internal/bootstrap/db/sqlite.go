package db

// SQLite support is intentionally disabled in the canonical demo. To enable
// it, add gorm.io/driver/sqlite to the module and uncomment the implementation
// below. Keep its names database-specific if PostgreSQL is also enabled.

/*
import (
	"fmt"
	"strings"
	"sync"

	"github.com/lanechi/gonex/config"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var sqliteDB struct {
	sync.RWMutex
	db *gorm.DB
}

func InitializeSQLite(configuration config.Config) error {
	if configuration == nil { return fmt.Errorf("sqlite configuration is nil") }
	dsn := strings.TrimSpace(configuration.GetString("DATABASE_DSN"))
	if dsn == "" { dsn = "data/app.db" }
	database, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil { return fmt.Errorf("open sqlite database: %w", err) }
	sqliteDB.Lock()
	defer sqliteDB.Unlock()
	if sqliteDB.db != nil { return fmt.Errorf("sqlite database is already initialized") }
	sqliteDB.db = database
	return nil
}

func SQLite() *gorm.DB {
	sqliteDB.RLock(); defer sqliteDB.RUnlock()
	if sqliteDB.db == nil { panic("sqlite database is not initialized") }
	return sqliteDB.db
}

func CloseSQLite() error {
	sqliteDB.Lock(); defer sqliteDB.Unlock()
	if sqliteDB.db == nil { return nil }
	sqlDB, err := sqliteDB.db.DB()
	if err != nil { return err }
	if err := sqlDB.Close(); err != nil { return err }
	sqliteDB.db = nil
	return nil
}
*/
