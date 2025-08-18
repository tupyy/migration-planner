package store

import (
	"context"
	"fmt"
	"hash/fnv"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	lockKeyFixed          = "zed_token_lock_key"
	globalLockStmtFixed   = "SELECT pg_try_advisory_lock($1);"
	sharedLockStmtFixed   = "SELECT pg_try_advisory_lock_shared($1);"
	globalUnlockStmtFixed = "SELECT pg_advisory_unlock($1);"
	sharedUnlockStmtFixed = "SELECT pg_advisory_unlock_shared($1);"
)

// ZedTokenStore is the corrected version of ZedTokenStore
type ZedTokenStore struct {
	lockID int32
	db     *gorm.DB
}

// NewZedTokenStore creates a new ZedTokenStoreFixed (made public for testing)
func NewZedTokenStore(db *gorm.DB) *ZedTokenStore {
	h := fnv.New32a()
	h.Write([]byte(lockKeyFixed))
	return &ZedTokenStore{db: db, lockID: int32(h.Sum32())}
}

// Read acquires a shared lock and reads the token
func (z *ZedTokenStore) Read(ctx context.Context) (*string, error) {
	if err := z.acquireLock(ctx, true); err != nil {
		return nil, fmt.Errorf("failed to acquire shared lock: %w", err)
	}

	defer func() {
		if err := z.releaseLock(ctx, true); err != nil {
			// Log error but don't fail the operation since we got the data
			// In production, this should use proper logging
			fmt.Printf("Warning: failed to release shared lock: %v\n", err)
		}
	}()

	var token string
	tx := z.getDB(ctx).Raw("SELECT token FROM zed_token LIMIT 1;").Scan(&token)
	if tx.Error != nil {
		return nil, fmt.Errorf("failed to read token: %w", tx.Error)
	}

	return &token, nil
}

// Write acquires a global lock and writes the token
func (z *ZedTokenStore) Write(ctx context.Context, token string) error {
	if err := z.acquireLock(ctx, false); err != nil {
		return fmt.Errorf("failed to acquire global lock: %w", err)
	}

	defer func() {
		if err := z.releaseLock(ctx, false); err != nil {
			zap.S().Errorw("failed to release gobal lock", "error", err)
		}
	}()

	tx := z.getDB(ctx).Exec("UPDATE zed_token SET token = ?;", token)
	if tx.Error != nil {
		return fmt.Errorf("failed to write token: %w", tx.Error)
	}

	return nil
}

// acquireLock attempts to acquire either a shared or global advisory lock
func (z *ZedTokenStore) acquireLock(ctx context.Context, isShared bool) error {
	lockStmt := globalLockStmtFixed
	if isShared {
		lockStmt = sharedLockStmtFixed
	}

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			var hasLock bool
			tx := z.db.WithContext(ctx).Raw(lockStmt, z.lockID).Scan(&hasLock)
			if tx.Error != nil {
				return fmt.Errorf("lock query failed: %w", tx.Error)
			}
			if hasLock {
				return nil
			}
		case <-ctx.Done():
			return fmt.Errorf("lock acquisition timeout: %w", ctx.Err())
		}
	}
}

// releaseLock releases either a shared or global advisory lock
func (z *ZedTokenStore) releaseLock(ctx context.Context, isShared bool) error {
	unlockStmt := globalUnlockStmtFixed
	if isShared {
		unlockStmt = sharedUnlockStmtFixed
	}

	var released bool
	tx := z.db.WithContext(ctx).Raw(unlockStmt, z.lockID).Scan(&released)
	if tx.Error != nil {
		return fmt.Errorf("unlock query failed: %w", tx.Error)
	}

	if !released {
		return fmt.Errorf("failed to release lock (lock was not held)")
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
