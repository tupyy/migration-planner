package authz

import (
	"context"

	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/pkg/migrations"
)

func init() {
	migrations.AutzMigrations.Register(2, initPlatform)
}

func initPlatform(ctx context.Context, authzSrv *service.Authz, assessmentSrv *service.AssessmentService) error {
	return nil
}
