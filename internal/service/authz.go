package service

import (
	"context"
	"fmt"
	"slices"

	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
)

type AuthzService struct {
	s store.Store
}

func NewAuthzService(s store.Store) *AuthzService {
	return &AuthzService{s: s}
}

func (a *AuthzService) CreateUser(ctx context.Context, user auth.User) error {
	return a.s.Authz().WriteRelationships(ctx, store.WithMemberRelationship(
		user.Username,
		user.EmailDomain,
	))
}

func (a *AuthzService) CreateAssessmentRelationship(ctx context.Context, assessmentId string, user auth.User) error {
	return a.s.Authz().WriteRelationships(ctx,
		store.WithOwnerRelationship(assessmentId, model.NewUserSubject(user.Username)),
		store.WithEditorRelationship(assessmentId, model.NewOrganizationSubject(user.EmailDomain)),
	)
}

func (a *AuthzService) ListAssessments(ctx context.Context, user auth.User) ([]string, error) {
	res, err := a.s.Authz().ListResources(ctx, user.Username)
	if err != nil {
		return []string{}, err
	}
	allowedAssessments := []string{}

	for _, r := range res {
		if slices.Contains(r.Permissions, model.ReadPermission) {
			allowedAssessments = append(allowedAssessments, r.AssessmentID)
		}
	}

	return allowedAssessments, nil
}

func (a *AuthzService) ShareAssessment(ctx context.Context, assessmentId string, userId *string, orgId *string) error {
	if userId == nil && orgId == nil {
		return fmt.Errorf("either userId or orgId must be present")
	}

	var subject model.Subject

	if userId != nil {
		subject = model.NewUserSubject(*userId)
	}
	if orgId != nil {
		subject = model.NewOrganizationSubject(*orgId)
	}
	return a.s.Authz().WriteRelationships(ctx, store.WithReaderRelationship(assessmentId, subject))
}

func (a *AuthzService) GetPermissions(ctx context.Context, assessmentId string, user auth.User) ([]model.Permission, error) {
	permissionsMap, err := a.s.Authz().GetPermissions(ctx, []string{assessmentId}, user.Username)
	if err != nil {
		return []model.Permission{}, err
	}

	if permissions, ok := permissionsMap[assessmentId]; ok {
		return permissions, nil
	}

	return []model.Permission{}, nil
}

func (a *AuthzService) GetBulkPermissions(ctx context.Context, assessmentIds []string, user auth.User) (map[string][]model.Permission, error) {
	return a.s.Authz().GetPermissions(ctx, assessmentIds, user.Username)
}

func (a *AuthzService) HasPermission(ctx context.Context, assessmentId string, user auth.User, permission model.Permission) (bool, error) {
	permissionsMap, err := a.s.Authz().GetPermissions(ctx, []string{assessmentId}, user.Username)
	if err != nil {
		return false, err
	}

	if permissions, ok := permissionsMap[assessmentId]; ok {
		return slices.Contains(permissions, permission), nil
	}

	return false, nil
}

func (a *AuthzService) DeleteAssessmentRelationship(ctx context.Context, assessmentId string) error {
	return a.s.Authz().DeleteRelationships(ctx, assessmentId)
}
