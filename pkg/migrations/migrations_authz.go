package migrations

import (
	"context"
	"maps"
	"slices"

	"github.com/kubev2v/migration-planner/internal/service"
)

var AutzMigrations = &Migrations{migrations: make(map[int]MigrationFunc)}

type MigrationFunc func(ctx context.Context, authzSrv any, assessmentSrv *service.AssessmentService) error

type Migrations struct {
	migrations map[int]MigrationFunc
}

func (m *Migrations) Register(id int, migrationFn MigrationFunc) {
	m.migrations[id] = migrationFn
}

func (m *Migrations) Migrate(ctx context.Context, authzSrv any, assessmentSrv *service.AssessmentService) error {
	if len(m.migrations) == 0 {
		return nil
	}
	keys := slices.Sorted(maps.Keys(m.migrations))

	for _, k := range keys {
		fn := m.migrations[k]
		if err := fn(ctx, authzSrv, assessmentSrv); err != nil {
			return err
		}
	}

	return nil
}
