package store

import (
	"context"
	"fmt"
	"hash/fnv"
	"sync"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	lockKeyFixed          = "zed_token_lock_key"
	globalLockStmtFixed   = "SELECT pg_advisory_lock(%d);"
	sharedLockStmtFixed   = "SELECT pg_advisory_lock_shared(%d);"
	globalUnlockStmtFixed = "SELECT pg_advisory_unlock(%d);"
	sharedUnlockStmtFixed = "SELECT pg_advisory_unlock_shared(%d);"
	writeStmt             = "INSERT INTO zed_token VALUES (1, '%s') ON CONFLICT (id) DO UPDATE SET token = excluded.token;"
)

// ZedTokenStore is the corrected version of ZedTokenStore
type ZedTokenStore struct {
	lockID   int32
	db       *gorm.DB
	lockConn *gorm.DB   // dedicated connection for lock operations
	mu       sync.Mutex // protects lock acquisition/release operations
}

// NewZedTokenStore creates a new ZedTokenStoreFixed (made public for testing)
func NewZedTokenStore(db *gorm.DB) *ZedTokenStore {
	h := fnv.New32a()
	h.Write([]byte(lockKeyFixed))

	// Create a dedicated connection for lock operations to ensure same session
	lockConn := createDedicatedConnection(db)

	return &ZedTokenStore{
		db:       db,
		lockConn: lockConn,
		lockID:   int32(h.Sum32()),
	}
}

// createDedicatedConnection creates a new GORM DB instance with a single connection
func createDedicatedConnection(db *gorm.DB) *gorm.DB {
	// Create new connection with the same dialector and config
	newDB, err := gorm.Open(db.Dialector, &gorm.Config{
		Logger: db.Config.Logger,
	})
	if err != nil {
		zap.S().Warnw("failed to create dedicated lock connection, using original", "error", err)
		return db
	}

	// Configure the connection pool to use only 1 connection
	newSqlDB, err := newDB.DB()
	if err != nil {
		zap.S().Warnw("failed to configure lock connection pool, using original", "error", err)
		return db
	}

	newSqlDB.SetMaxOpenConns(1)
	newSqlDB.SetMaxIdleConns(1)
	newSqlDB.SetConnMaxLifetime(0) // connections never expire

	return newDB
}

// Read reads the token (assumes caller has already acquired appropriate lock)
func (z *ZedTokenStore) Read(ctx context.Context) (*string, error) {
	var token string
	tx := z.getDB(ctx).Raw("SELECT token FROM zed_token LIMIT 1;").Scan(&token)
	if tx.Error != nil {
		return nil, fmt.Errorf("failed to read token: %w", tx.Error)
	}

	return &token, nil
}

// Write writes the token (assumes caller has already acquired appropriate lock)
func (z *ZedTokenStore) Write(ctx context.Context, token string) error {
	// upsert query to keep only one value of the token in the db
	tx := z.getDB(ctx).Exec(fmt.Sprintf(writeStmt, token))
	if tx.Error != nil {
		zap.S().Errorf(tx.Error.Error())
		return fmt.Errorf("failed to write token: %w", tx.Error)
	}

	return nil
}

// AcquireLock attempts to acquire either a shared or global advisory lock
func (z *ZedTokenStore) AcquireLock(ctx context.Context, isShared bool) error {
	lockStmt := globalLockStmtFixed
	if isShared {
		lockStmt = sharedLockStmtFixed
	}

	lockQuery := fmt.Sprintf(lockStmt, z.lockID)
	// Use dedicated connection to ensure same session for lock/unlock
	tx := z.lockConn.WithContext(ctx).Exec(lockQuery)
	if tx.Error != nil {
		return fmt.Errorf("lock query failed: %w", tx.Error)
	}

	return nil
}

// ReleaseLock releases either a shared or global advisory lock
func (z *ZedTokenStore) ReleaseLock(ctx context.Context, isShared bool) error {
	unlockStmt := globalUnlockStmtFixed
	if isShared {
		unlockStmt = sharedUnlockStmtFixed
	}

	var released bool
	unlockQuery := fmt.Sprintf(unlockStmt, z.lockID)
	// Use same dedicated connection as AcquireLock to ensure same session
	tx := z.lockConn.WithContext(ctx).Raw(unlockQuery).Scan(&released)
	if tx.Error != nil {
		return fmt.Errorf("unlock query failed: %w", tx.Error)
	}

	if !released {
		return fmt.Errorf("failed to release lock (lock was not held). Lock is shared: %v", isShared)
	}

	return nil
}

// getDB returns the database connection, preferring transaction from context
func (z *ZedTokenStore) getDB(ctx context.Context) *gorm.DB {
	tx := FromContext(ctx)
	if tx != nil {
		return tx
	}
	return z.db
}
