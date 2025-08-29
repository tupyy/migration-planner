package service

import (
	"context"
	"fmt"
	"slices"

	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
)

const (
	superOrg = "redhat_consultant" // To Be Defined
)

// AuthzService provides authorization functionality. All methods require transactions
// because the zed_token store uses PostgreSQL advisory transaction locks to ensure consistency
// between SpiceDB operations and the stored zed token.
//
// Lock Types:
//   - Global Lock (Exclusive): pg_advisory_xact_lock() - Used for write operations (WriteRelationships, DeleteRelationships)
//     that modify relationships. Only one global lock can be held at a time.
//   - Shared Lock: pg_advisory_xact_lock_shared() - Used for read operations (ListResources, GetPermissions)
//     that query permissions. Multiple shared locks can be held concurrently, but not with a global lock.
//
// The advisory locks are transaction-scoped and automatically released when the transaction
// commits or rolls back, ensuring proper cleanup and preventing deadlocks.
type AuthzService struct {
	s                     store.Store
	conditionalGenerators []*ConditionalRelationshipGenerator
}

func NewAuthzService(s store.Store) *AuthzService {
	generators := []*ConditionalRelationshipGenerator{
		NewSuperOrgRelationGen(superOrg),
	}
	return &AuthzService{
		s:                     s,
		conditionalGenerators: generators,
	}
}

// CreateUser creates a new user in the authorization system.
// Acquires: Global Lock (Exclusive) - writes relationships to SpiceDB.
func (a *AuthzService) CreateUser(ctx context.Context, user auth.User) error {
	// absolutly, we need a transaction here for the lock to be acquired
	ctx, err := a.s.NewTransactionContext(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

	relationships := []model.RelationshipFn{
		store.WithMemberRelationship(user.Username, user.EmailDomain),
	}

	if err := a.s.Authz().WriteRelationships(ctx, relationships...); err != nil {
		return err
	}

	if _, err := store.Commit(ctx); err != nil {
		return err
	}

	return nil
}

// CreateAssessmentRelationship creates ownership and editor relationships for an assessment.
// Acquires: Global Lock (Exclusive) - writes relationships to SpiceDB.
func (a *AuthzService) CreateAssessmentRelationship(ctx context.Context, assessmentId string, user auth.User) error {
	ctx, err := a.s.NewTransactionContext(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

	relationships := []model.RelationshipFn{
		store.WithOwnerRelationship(assessmentId, model.NewUserSubject(user.Username)),
		store.WithEditorRelationship(assessmentId, model.NewOrganizationSubject(user.EmailDomain)),
	}

	for _, gen := range a.conditionalGenerators {
		relationships = append(relationships, gen.Generate(user, assessmentId)...)
	}

	if err := a.s.Authz().WriteRelationships(ctx, relationships...); err != nil {
		return err
	}

	if _, err := store.Commit(ctx); err != nil {
		return err
	}

	return nil
}

// ListAssessments returns a list of assessment IDs that the user has read access to.
// Acquires: Shared Lock - reads from SpiceDB using stored zed token.
func (a *AuthzService) ListAssessments(ctx context.Context, user auth.User) ([]string, error) {
	ctx, err := a.s.NewTransactionContext(ctx)
	if err != nil {
		return []string{}, err
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

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

	if _, err := store.Commit(ctx); err != nil {
		return []string{}, err
	}

	return allowedAssessments, nil
}

// ShareAssessment grants read access to an assessment for a user or organization.
// Acquires: Global Lock (Exclusive) - writes relationships to SpiceDB.
func (a *AuthzService) ShareAssessment(ctx context.Context, assessmentId string, userId *string, orgId *string) error {
	ctx, err := a.s.NewTransactionContext(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

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

	if err := a.s.Authz().WriteRelationships(ctx, store.WithReaderRelationship(assessmentId, subject)); err != nil {
		return err
	}

	if _, err := store.Commit(ctx); err != nil {
		return err
	}

	return nil
}

// GetPermissions returns the permissions a user has for a specific assessment.
// Acquires: Shared Lock - reads from SpiceDB using stored zed token.
func (a *AuthzService) GetPermissions(ctx context.Context, assessmentId string, user auth.User) ([]model.Permission, error) {
	ctx, err := a.s.NewTransactionContext(ctx)
	if err != nil {
		return []model.Permission{}, err
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

	permissionsMap, err := a.s.Authz().GetPermissions(ctx, []string{assessmentId}, user.Username)
	if err != nil {
		return []model.Permission{}, err
	}

	if permissions, ok := permissionsMap[assessmentId]; ok {
		if _, err := store.Commit(ctx); err != nil {
			return []model.Permission{}, err
		}
		return permissions, nil
	}

	if _, err := store.Commit(ctx); err != nil {
		return []model.Permission{}, err
	}

	return []model.Permission{}, nil
}

// GetBulkPermissions returns the permissions a user has for multiple assessments.
// Acquires: Shared Lock - reads from SpiceDB using stored zed token.
func (a *AuthzService) GetBulkPermissions(ctx context.Context, assessmentIds []string, user auth.User) (map[string][]model.Permission, error) {
	ctx, err := a.s.NewTransactionContext(ctx)
	if err != nil {
		return map[string][]model.Permission{}, err
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

	permissions, err := a.s.Authz().GetPermissions(ctx, assessmentIds, user.Username)
	if err != nil {
		return map[string][]model.Permission{}, err
	}

	if _, err := store.Commit(ctx); err != nil {
		return map[string][]model.Permission{}, err
	}

	return permissions, nil
}

// HasPermission checks if a user has a specific permission for an assessment.
// Acquires: Shared Lock - reads from SpiceDB using stored zed token.
func (a *AuthzService) HasPermission(ctx context.Context, assessmentId string, user auth.User, permission model.Permission) (bool, error) {
	ctx, err := a.s.NewTransactionContext(ctx)
	if err != nil {
		return false, err
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

	permissionsMap, err := a.s.Authz().GetPermissions(ctx, []string{assessmentId}, user.Username)
	if err != nil {
		return false, err
	}

	if permissions, ok := permissionsMap[assessmentId]; ok {
		if _, err := store.Commit(ctx); err != nil {
			return false, err
		}
		return slices.Contains(permissions, permission), nil
	}

	if _, err := store.Commit(ctx); err != nil {
		return false, err
	}

	return false, nil
}

// DeleteAssessmentRelationship removes all relationships for an assessment.
// Acquires: Global Lock (Exclusive) - deletes relationships from SpiceDB.
func (a *AuthzService) DeleteAssessmentRelationship(ctx context.Context, assessmentId string) error {
	ctx, err := a.s.NewTransactionContext(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

	if err := a.s.Authz().DeleteRelationships(ctx, assessmentId); err != nil {
		return err
	}

	if _, err := store.Commit(ctx); err != nil {
		return err
	}

	return nil
}

// ConditionalRelationshipGenerator generates RelationshipFn based on a condition applied to the user model.
// It allows for dynamic relationship creation where certain users (based on their properties like organization,
// role, or other attributes) automatically receive specific permissions or relationships.
//
// Example usage:
//   - Super organization users get automatic owner permissions
//   - Admin users get automatic editor permissions
//   - Users from specific domains get automatic reader permissions
type ConditionalRelationshipGenerator struct {
	condition   func(user auth.User) bool
	templateFns []func(assessmentId string, user auth.User) model.RelationshipFn
}

func NewConditionalRelationship() *ConditionalRelationshipGenerator {
	return &ConditionalRelationshipGenerator{
		templateFns: make([]func(assessmentId string, user auth.User) model.RelationshipFn, 0),
	}
}

func (c *ConditionalRelationshipGenerator) WithCondition(fn func(auth.User) bool) *ConditionalRelationshipGenerator {
	c.condition = fn
	return c
}

func (c *ConditionalRelationshipGenerator) WithRelationshipTemplate(templateFn func(assessmentId string, user auth.User) model.RelationshipFn) *ConditionalRelationshipGenerator {
	c.templateFns = append(c.templateFns, templateFn)
	return c
}

func (c *ConditionalRelationshipGenerator) Generate(user auth.User, assessmentId string) []model.RelationshipFn {
	if !c.condition(user) {
		return []model.RelationshipFn{}
	}

	relationshipFn := []model.RelationshipFn{}

	for _, fn := range c.templateFns {
		relationshipFn = append(relationshipFn, fn(assessmentId, user))
	}

	return relationshipFn
}

func NewSuperOrgRelationGen(superOrgID string) *ConditionalRelationshipGenerator {
	return NewConditionalRelationship().
		WithCondition(func(u auth.User) bool {
			return u.Organization == superOrgID
		}).
		WithRelationshipTemplate(func(assessmentId string, user auth.User) model.RelationshipFn {
			return store.WithOwnerRelationship(assessmentId, model.NewOrganizationSubject(superOrgID))
		})
}
