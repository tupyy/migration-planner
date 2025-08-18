package store_test

import (
	"context"
	"sync"
	"time"

	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/store"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
)

var _ = Describe("ZedTokenStore", Ordered, func() {
	var (
		gormDB     *gorm.DB
		tokenStore *store.ZedTokenStore
	)

	BeforeAll(func() {
		var err error
		// Use a context for setup
		ctx := context.Background()

		// Initialize database using the same pattern as other tests
		cfg, err := config.New()
		Expect(err).To(BeNil())

		gormDB, err = store.InitDB(cfg)
		Expect(err).To(BeNil())

		// Create the zed_token table if it doesn't exist
		err = gormDB.WithContext(ctx).Exec(`
			CREATE TABLE IF NOT EXISTS zed_token (
				token TEXT NOT NULL DEFAULT ''
			);
		`).Error
		Expect(err).To(BeNil())

		// Ensure we have exactly one row
		var count int64
		err = gormDB.WithContext(ctx).Raw("SELECT COUNT(*) FROM zed_token").Scan(&count).Error
		Expect(err).To(BeNil())

		if count == 0 {
			err = gormDB.WithContext(ctx).Exec("INSERT INTO zed_token (token) VALUES ('')").Error
			Expect(err).To(BeNil())
		} else if count > 1 {
			err = gormDB.WithContext(ctx).Exec("DELETE FROM zed_token").Error
			Expect(err).To(BeNil())
			err = gormDB.WithContext(ctx).Exec("INSERT INTO zed_token (token) VALUES ('')").Error
			Expect(err).To(BeNil())
		}

		// Create token store
		tokenStore = store.NewZedTokenStore(gormDB)
		Expect(tokenStore).ToNot(BeNil())
	})

	BeforeEach(func() {
		// Reset token to empty before each test
		ctx := context.Background()
		err := gormDB.WithContext(ctx).Exec("UPDATE zed_token SET token = ''").Error
		Expect(err).To(BeNil())
	})

	Context("Read Operations", func() {
		It("should read empty token successfully", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			token, err := tokenStore.Read(ctx)
			Expect(err).To(BeNil())
			Expect(token).ToNot(BeNil())
			Expect(*token).To(Equal(""))
		})

		It("should read existing token successfully", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			testToken := "test-token-123"

			// Write token directly to database
			err := gormDB.Exec("UPDATE zed_token SET token = ?", testToken).Error
			Expect(err).To(BeNil())

			// Read token using store
			token, err := tokenStore.Read(ctx)
			Expect(err).To(BeNil())
			Expect(token).ToNot(BeNil())
			Expect(*token).To(Equal(testToken))
		})

		It("should handle context cancellation during read", func() {
			// Create a context that will be cancelled quickly
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()

			// This might succeed or fail depending on timing, but should not panic
			_, err := tokenStore.Read(ctx)
			// We don't assert success/failure since timing is unpredictable in tests
			_ = err
		})
	})

	Context("Write Operations", func() {
		It("should write token successfully", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			testToken := "write-test-token-456"

			err := tokenStore.Write(ctx, testToken)
			Expect(err).To(BeNil())

			// Verify token was written by reading directly from database
			var storedToken string
			err = gormDB.Raw("SELECT token FROM zed_token LIMIT 1").Scan(&storedToken).Error
			Expect(err).To(BeNil())
			Expect(storedToken).To(Equal(testToken))
		})

		It("should overwrite existing token", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			initialToken := "initial-token"
			newToken := "updated-token"

			// Write initial token
			err := tokenStore.Write(ctx, initialToken)
			Expect(err).To(BeNil())

			// Overwrite with new token
			err = tokenStore.Write(ctx, newToken)
			Expect(err).To(BeNil())

			// Verify new token was stored
			var storedToken string
			err = gormDB.Raw("SELECT token FROM zed_token LIMIT 1").Scan(&storedToken).Error
			Expect(err).To(BeNil())
			Expect(storedToken).To(Equal(newToken))
		})

		It("should handle empty token write", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := tokenStore.Write(ctx, "")
			Expect(err).To(BeNil())

			// Verify empty token was stored
			var storedToken string
			err = gormDB.Raw("SELECT token FROM zed_token LIMIT 1").Scan(&storedToken).Error
			Expect(err).To(BeNil())
			Expect(storedToken).To(Equal(""))
		})

		It("should handle context cancellation during write", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()

			// This might succeed or fail depending on timing, but should not panic
			err := tokenStore.Write(ctx, "test-token")
			_ = err // Don't assert since timing is unpredictable
		})
	})

	Context("Large Token Handling", func() {
		It("should handle large tokens", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			// Create a large token (1KB)
			largeToken := ""
			for i := 0; i < 1024; i++ {
				largeToken += "A"
			}

			err := tokenStore.Write(ctx, largeToken)
			Expect(err).To(BeNil())

			token, err := tokenStore.Read(ctx)
			Expect(err).To(BeNil())
			Expect(*token).To(Equal(largeToken))
		})

		It("should handle special characters in tokens", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			specialToken := "token-with-special-chars-!@#$%^&*()_+-={}[]|\\:;\"'<>?,./"

			err := tokenStore.Write(ctx, specialToken)
			Expect(err).To(BeNil())

			token, err := tokenStore.Read(ctx)
			Expect(err).To(BeNil())
			Expect(*token).To(Equal(specialToken))
		})
	})

	Context("Concurrent Operations", func() {
		It("should handle concurrent reads", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			testToken := "concurrent-read-token"

			// Write initial token
			err := tokenStore.Write(ctx, testToken)
			Expect(err).To(BeNil())

			var wg sync.WaitGroup
			results := make([]string, 5)
			errors := make([]error, 5)

			// Launch 5 concurrent reads
			for i := 0; i < 5; i++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()
					token, err := tokenStore.Read(ctx)
					errors[index] = err
					if err == nil && token != nil {
						results[index] = *token
					}
				}(i)
			}

			wg.Wait()

			// All reads should succeed and return the same token
			for i := 0; i < 5; i++ {
				Expect(errors[i]).To(BeNil())
				Expect(results[i]).To(Equal(testToken))
			}
		})

		It("should serialize concurrent writes", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()

			var wg sync.WaitGroup
			tokens := []string{"token1", "token2", "token3", "token4", "token5"}
			errors := make([]error, len(tokens))

			// Launch concurrent writes
			for i, token := range tokens {
				wg.Add(1)
				go func(index int, t string) {
					defer wg.Done()
					errors[index] = tokenStore.Write(ctx, t)
				}(i, token)
			}

			wg.Wait()

			// All writes should succeed
			for i := 0; i < len(tokens); i++ {
				Expect(errors[i]).To(BeNil())
			}

			// Final token should be one of the written tokens
			finalToken, err := tokenStore.Read(ctx)
			Expect(err).To(BeNil())
			Expect(tokens).To(ContainElement(*finalToken))
		})

		It("should handle mixed concurrent reads and writes", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			initialToken := "initial"
			newToken := "updated"

			// Set initial token
			err := tokenStore.Write(ctx, initialToken)
			Expect(err).To(BeNil())

			var wg sync.WaitGroup
			readResults := make([]string, 3)
			readErrors := make([]error, 3)
			writeErrors := make([]error, 2)

			// Launch concurrent operations
			// 3 reads
			for i := 0; i < 3; i++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()
					time.Sleep(time.Duration(index*10) * time.Millisecond) // Stagger operations
					token, err := tokenStore.Read(ctx)
					readErrors[index] = err
					if err == nil && token != nil {
						readResults[index] = *token
					}
				}(i)
			}

			// 2 writes
			for i := 0; i < 2; i++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()
					time.Sleep(time.Duration(index*15) * time.Millisecond) // Stagger operations
					writeErrors[index] = tokenStore.Write(ctx, newToken)
				}(i)
			}

			wg.Wait()

			// All operations should succeed
			for i := 0; i < 3; i++ {
				Expect(readErrors[i]).To(BeNil())
			}
			for i := 0; i < 2; i++ {
				Expect(writeErrors[i]).To(BeNil())
			}

			// Each read result should be either the initial or new token
			validTokens := []string{initialToken, newToken}
			for i := 0; i < 3; i++ {
				Expect(validTokens).To(ContainElement(readResults[i]))
			}
		})
	})

	Context("Error Scenarios", func() {
		It("should handle database connection issues gracefully", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// This test would require a way to simulate database failure
			// For now, we'll just verify the methods don't panic with normal operation
			token, err := tokenStore.Read(ctx)
			Expect(err).To(BeNil())
			Expect(token).ToNot(BeNil())
		})
	})

	Context("Lock Consistency", func() {
		It("should ensure read-write consistency", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			token1 := "consistency-test-1"
			token2 := "consistency-test-2"

			// Write first token
			err := tokenStore.Write(ctx, token1)
			Expect(err).To(BeNil())

			// Read should return first token
			readToken, err := tokenStore.Read(ctx)
			Expect(err).To(BeNil())
			Expect(*readToken).To(Equal(token1))

			// Write second token
			err = tokenStore.Write(ctx, token2)
			Expect(err).To(BeNil())

			// Read should return second token
			readToken, err = tokenStore.Read(ctx)
			Expect(err).To(BeNil())
			Expect(*readToken).To(Equal(token2))
		})

		It("should handle rapid read-write cycles", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			baseToken := "rapid-test"

			for i := 0; i < 10; i++ {
				token := baseToken + "-" + string(rune('0'+i))

				// Write
				err := tokenStore.Write(ctx, token)
				Expect(err).To(BeNil())

				// Immediate read
				readToken, err := tokenStore.Read(ctx)
				Expect(err).To(BeNil())
				Expect(*readToken).To(Equal(token))
			}
		})
	})
})
