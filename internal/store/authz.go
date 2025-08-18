package store

import (
	"context"
	"io"

	v1pb "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/authzed-go/v1"
	"github.com/kubev2v/migration-planner/internal/store/model"
)

type ZedTokenKey struct{}

type Authz interface {
	WriteRelationships(ctx context.Context, relationships ...model.RelationshipFn) error
	DeleteRelationships(ctx context.Context, relationships ...model.RelationshipFn) error
	ListResources(ctx context.Context, userID string) ([]model.Resource, error)
	HasPermissions(ctx context.Context, userID string, assessmentID string, permissions []model.Permission) (map[model.Permission]bool, error)
}

type AuthzStore struct {
	ZedToken *v1pb.ZedToken // should be public to allow unit test to use it
	client   *authzed.Client
	zedStore *ZedTokenStore
}

func NewAuthzStore(zedTokenStore *ZedTokenStore, client *authzed.Client) Authz {
	return &AuthzStore{
		client:   client,
		zedStore: zedTokenStore,
	}
}

// WriteRelationships writes multiple relationships to SpiceDB using relationship functions.
// This method allows batch creation of relationships for better performance and atomicity.
//
// Parameters:
//   - ctx: Context for the request
//   - relationships: Variadic list of RelationshipFn functions that define relationships to create
//
// Returns:
//   - error: Error if the operation fails, nil on success
//
// Example:
//
//	userSubject := model.Subject{Kind: model.User, Id: "user123"}
//	orgSubject := model.Subject{Kind: model.Organization, Id: "org456"}
//
//	err := authzService.WriteRelationships(ctx,
//	    AddUserToOrganization("user123", "org456"),
//	    WithOwnerRelationship("assessment789", userSubject),
//	    WithReaderRelationship("assessment789", orgSubject),
//	)
//	if err != nil {
//	    log.Printf("Failed to write relationships: %v", err)
//	}
func (a *AuthzStore) WriteRelationships(ctx context.Context, relationships ...model.RelationshipFn) error {
	zedToken, err := a.writeRelationships(ctx, relationships...)
	a.ZedToken = zedToken
	return err
}

// writeRelationships is the private implementation of WriteRelationships
func (a *AuthzStore) writeRelationships(ctx context.Context, relationships ...model.RelationshipFn) (*v1pb.ZedToken, error) {
	relationshipsUpdate := []*v1pb.RelationshipUpdate{}

	if len(relationships) == 0 {
		return nil, nil
	}

	for _, fn := range relationships {
		relationshipsUpdate = fn(relationshipsUpdate)
	}

	resp, err := a.client.WriteRelationships(ctx, &v1pb.WriteRelationshipsRequest{
		Updates: relationshipsUpdate,
	})
	if err != nil {
		return nil, err
	}

	return resp.WrittenAt, nil
}

// DeleteRelationships removes multiple relationships from SpiceDB using relationship functions.
// This method takes relationship functions (normally used for creation) and converts them to
// deletion operations, allowing batch removal of relationships.
//
// Parameters:
//   - ctx: Context for the request
//   - relationships: Variadic list of RelationshipFn functions that define relationships to delete
//
// Returns:
//   - error: Error if the operation fails, nil on success
//
// Example:
//
//	userSubject := model.NewUserSubject("user123")
//
//	err := authzService.DeleteRelationships(ctx,
//	    WithOwnerRelationship("assessment789", userSubject),
//	    WithReaderRelationship("assessment456", userSubject),
//	)
//	if err != nil {
//	    log.Printf("Failed to delete relationships: %v", err)
//	}
func (a *AuthzStore) DeleteRelationships(ctx context.Context, relationships ...model.RelationshipFn) error {
	zedToken, err := a.deleteRelationships(ctx, relationships...)
	a.ZedToken = zedToken
	return err
}

// deleteRelationships is the private implementation of DeleteRelationships
func (a *AuthzStore) deleteRelationships(ctx context.Context, relationships ...model.RelationshipFn) (*v1pb.ZedToken, error) {
	relationshipsUpdate := []*v1pb.RelationshipUpdate{}

	if len(relationships) == 0 {
		return nil, nil
	}

	for _, fn := range relationships {
		relationshipsUpdate = fn(relationshipsUpdate)
	}

	// change op for each relationships
	for _, r := range relationshipsUpdate {
		r.Operation = v1pb.RelationshipUpdate_OPERATION_DELETE
	}

	resp, err := a.client.WriteRelationships(ctx, &v1pb.WriteRelationshipsRequest{
		Updates: relationshipsUpdate,
	})
	if err != nil {
		return nil, err
	}

	return resp.WrittenAt, nil
}

// ListResources returns a list of resources (assessments) that the user has access to with their permissions.
// This method discovers all assessments the user can read and determines their full permission set for each.
//
// Parameters:
//   - ctx: Context for the request
//   - userID: The ID of the user to check resources for
//
// Returns:
//   - []model.Resource: A slice of resources with their associated permissions
//   - error: Error if the operation fails, nil on success
//
// Example:
//
//	resources, err := authzService.ListResources(ctx, "user123")
//	if err != nil {
//	    log.Printf("Failed to list resources: %v", err)
//	    return
//	}
//
//	for _, resource := range resources {
//	    fmt.Printf("Assessment %s has permissions: ", resource.AssessmentID)
//	    for _, perm := range resource.Permissions {
//	        fmt.Printf("%s ", perm.String())
//	    }
//	    fmt.Println()
//	}
func (a *AuthzStore) ListResources(ctx context.Context, userID string) ([]model.Resource, error) {
	return a.listResources(ctx, userID)
}

// listResources is the private implementation of ListResources
func (a *AuthzStore) listResources(ctx context.Context, userID string) ([]model.Resource, error) {
	// try to get zedToken from ctx if any
	token := a.getZedToken(ctx)

	// Lookup resources for which the user has at least read permission
	req := &v1pb.LookupResourcesRequest{
		ResourceObjectType: model.AssessmentObject,
		Permission:         model.ReadPermission.String(),
		Subject: &v1pb.SubjectReference{
			Object: &v1pb.ObjectReference{
				ObjectType: model.UserObject,
				ObjectId:   userID,
			},
		},
	}

	// Use token for at least as fresh consistency
	if token != nil {
		req.Consistency = &v1pb.Consistency{
			Requirement: &v1pb.Consistency_AtLeastAsFresh{
				AtLeastAsFresh: token,
			},
		}
	}

	resp, err := a.client.LookupResources(ctx, req)
	if err != nil {
		return nil, err
	}

	var resources []model.Resource
	for {
		resource, err := resp.Recv()
		if err != nil {
			// Check if we've reached the end of the stream
			if err == io.EOF {
				break
			}
			return nil, err
		}

		// Get all permissions for this assessment
		permissions, err := a.getPermissions(ctx, userID, resource.ResourceObjectId)
		if err != nil {
			return nil, err
		}

		resources = append(resources, model.Resource{
			AssessmentID: resource.ResourceObjectId,
			Permissions:  permissions,
		})
	}

	return resources, nil
}

// HasPermissions checks if a user has multiple permissions on an assessment in one call.
// This method is more efficient than calling individual permission checks when you need to
// verify multiple permissions for the same user and resource.
//
// Parameters:
//   - ctx: Context for the request
//   - userID: The ID of the user to check permissions for
//   - assessmentID: The ID of the assessment to check permissions on
//   - permissions: A slice of permissions to check
//
// Returns:
//   - map[model.Permission]bool: A map where keys are permissions and values indicate whether the user has them
//   - error: Error if the operation fails, nil on success
//
// Example:
//
//	permissionsToCheck := []model.Permission{
//	    model.ReadPermission,
//	    model.EditPermission,
//	    model.DeletePermission,
//	}
//
//	result, err := authzService.HasPermissions(ctx, "user123", "assessment789", permissionsToCheck)
//	if err != nil {
//	    log.Printf("Failed to check permissions: %v", err)
//	    return
//	}
func (a *AuthzStore) HasPermissions(ctx context.Context, userID string, assessmentID string, permissions []model.Permission) (map[model.Permission]bool, error) {
	return a.hasPermissions(ctx, userID, assessmentID, permissions)
}

// hasPermissions is the private implementation of HasPermissions
func (a *AuthzStore) hasPermissions(ctx context.Context, userID string, assessmentID string, permissions []model.Permission) (map[model.Permission]bool, error) {
	if len(permissions) == 0 {
		return make(map[model.Permission]bool), nil
	}

	// Build bulk permission check request
	var items []*v1pb.CheckBulkPermissionsRequestItem
	for _, perm := range permissions {
		items = append(items, &v1pb.CheckBulkPermissionsRequestItem{
			Resource: &v1pb.ObjectReference{
				ObjectType: model.AssessmentObject,
				ObjectId:   assessmentID,
			},
			Permission: perm.String(),
			Subject: &v1pb.SubjectReference{
				Object: &v1pb.ObjectReference{
					ObjectType: model.UserObject,
					ObjectId:   userID,
				},
			},
		})
	}

	token := a.getZedToken(ctx)
	req := &v1pb.CheckBulkPermissionsRequest{Items: items}
	if token != nil {
		req.Consistency = &v1pb.Consistency{
			Requirement: &v1pb.Consistency_AtLeastAsFresh{
				AtLeastAsFresh: token,
			},
		}
	}

	resp, err := a.client.CheckBulkPermissions(ctx, req)
	if err != nil {
		return nil, err
	}

	// Build result map
	result := make(map[model.Permission]bool)
	for i, pair := range resp.Pairs {
		// Check if the response is an item (not an error)
		if item := pair.GetItem(); item != nil {
			result[permissions[i]] = item.Permissionship == v1pb.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION
		} else {
			// If there's an error for this permission check, set it to false
			result[permissions[i]] = false
		}
	}

	return result, nil
}

// getPermissions checks all possible permissions for a user on an assessment using bulk check
func (a *AuthzStore) getPermissions(ctx context.Context, userID string, assessmentID string) ([]model.Permission, error) {
	token := a.getZedToken(ctx)

	allPermissions := []model.Permission{
		model.ReadPermission,
		model.EditPermission,
		model.SharePermission,
		model.DeletePermission,
	}

	// Build bulk permission check request
	var items []*v1pb.CheckBulkPermissionsRequestItem
	for _, perm := range allPermissions {
		items = append(items, &v1pb.CheckBulkPermissionsRequestItem{
			Resource: &v1pb.ObjectReference{
				ObjectType: model.AssessmentObject,
				ObjectId:   assessmentID,
			},
			Permission: perm.String(),
			Subject: &v1pb.SubjectReference{
				Object: &v1pb.ObjectReference{
					ObjectType: model.UserObject,
					ObjectId:   userID,
				},
			},
		})
	}

	req := &v1pb.CheckBulkPermissionsRequest{Items: items}
	if token != nil {
		req.Consistency = &v1pb.Consistency{
			Requirement: &v1pb.Consistency_AtLeastAsFresh{
				AtLeastAsFresh: token,
			},
		}
	}
	resp, err := a.client.CheckBulkPermissions(ctx, req)
	if err != nil {
		return nil, err
	}

	var userPermissions []model.Permission
	for i, pair := range resp.Pairs {
		// Check if the response is an item (not an error)
		if item := pair.GetItem(); item != nil {
			if item.Permissionship == v1pb.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION {
				userPermissions = append(userPermissions, allPermissions[i])
			}
		}
		// If there's an error for this permission check, we skip it
		// (could optionally log the error via pair.GetError())
	}

	return userPermissions, nil
}

func (a *AuthzStore) getZedToken(ctx context.Context) *v1pb.ZedToken {
	// check if service has the token already
	if a.ZedToken != nil {
		return a.ZedToken
	}
	// look into the context (used for testing mainly)
	val := ctx.Value(ZedTokenKey{})
	if val == nil {
		return nil
	}
	token, ok := val.(*v1pb.ZedToken)
	if ok {
		return token
	}
	return nil
}

// WithOwnerRelationship creates a relationship function that adds an owner relationship.
// Owner relationships typically grant full control over an assessment, including the ability
// to share, edit, and delete the assessment.
//
// Parameters:
//   - assessmentID: The ID of the assessment to grant ownership on
//   - subject: The subject (user or organization) to grant ownership to
//
// Returns:
//   - model.RelationshipFn: A function that can be used with WriteRelationships or DeleteRelationships
//
// Example:
//
//	userSubject := model.NewUserSubject("user123")
//	ownershipFn := WithOwnerRelationship("assessment789", userSubject)
//
//	err := authzService.WriteRelationships(ctx, ownershipFn)
//	if err != nil {
//	    log.Printf("Failed to grant ownership: %v", err)
//	}
func WithOwnerRelationship(assessmentID string, subject model.Subject) model.RelationshipFn {
	return func(updates []*v1pb.RelationshipUpdate) []*v1pb.RelationshipUpdate {
		relationshipUpdate := &v1pb.RelationshipUpdate{
			Operation: v1pb.RelationshipUpdate_OPERATION_TOUCH,
			Relationship: &v1pb.Relationship{
				Resource: &v1pb.ObjectReference{
					ObjectType: model.AssessmentObject,
					ObjectId:   assessmentID,
				},
				Relation: model.OwnerRelationshipKind.String(),
				Subject: &v1pb.SubjectReference{
					Object: &v1pb.ObjectReference{
						ObjectType: subject.Kind.String(),
						ObjectId:   subject.Id,
					},
				},
			},
		}

		if subject.Kind == model.Organization {
			relationshipUpdate.Relationship.Subject.OptionalRelation = model.MemberRelationshipKind.String()
		}

		return append(updates, relationshipUpdate)
	}
}

// WithReaderRelationship creates a relationship function that adds a reader relationship.
// Reader relationships typically grant read-only access to an assessment.
//
// Parameters:
//   - assessmentID: The ID of the assessment to grant read access on
//   - subject: The subject (user or organization) to grant read access to
//
// Returns:
//   - model.RelationshipFn: A function that can be used with WriteRelationships or DeleteRelationships
//
// Example:
//
//	orgSubject := model.NewOrganizationSubject("org456")
//	readerFn := WithReaderRelationship("assessment789", orgSubject)
//
//	err := authzService.WriteRelationships(ctx, readerFn)
//	if err != nil {
//	    log.Printf("Failed to grant read access: %v", err)
//	}
func WithReaderRelationship(assessmentID string, subject model.Subject) model.RelationshipFn {
	return func(updates []*v1pb.RelationshipUpdate) []*v1pb.RelationshipUpdate {
		relationshipUpdate := &v1pb.RelationshipUpdate{
			Operation: v1pb.RelationshipUpdate_OPERATION_TOUCH,
			Relationship: &v1pb.Relationship{
				Resource: &v1pb.ObjectReference{
					ObjectType: model.AssessmentObject,
					ObjectId:   assessmentID,
				},
				Relation: model.ReaderRelationshipKind.String(),
				Subject: &v1pb.SubjectReference{
					Object: &v1pb.ObjectReference{
						ObjectType: subject.Kind.String(),
						ObjectId:   subject.Id,
					},
				},
			},
		}

		if subject.Kind == model.Organization {
			relationshipUpdate.Relationship.Subject.OptionalRelation = model.MemberRelationshipKind.String()
		}

		return append(updates, relationshipUpdate)
	}
}

// WithEditorRelationship creates a relationship function that adds an editor relationship.
// Editor relationships typically grant read, edit access to an assessment, and add the
// ability to share it but not delete it.
//
// Parameters:
//   - assessmentID: The ID of the assessment to grant edit access on
//   - subject: The subject (user or organization) to grant edit access to
//
// Returns:
//   - model.RelationshipFn: A function that can be used with WriteRelationships or DeleteRelationships
//
// Example:
//
//	userSubject := model.NewUserSubject("user789")
//	editorFn := WithEditorRelationship("assessment123", userSubject)
//
//	err := authzService.WriteRelationships(ctx, editorFn)
//	if err != nil {
//	    log.Printf("Failed to grant edit access: %v", err)
//	}
func WithEditorRelationship(assessmentID string, subject model.Subject) model.RelationshipFn {
	return func(updates []*v1pb.RelationshipUpdate) []*v1pb.RelationshipUpdate {
		relationshipUpdate := &v1pb.RelationshipUpdate{
			Operation: v1pb.RelationshipUpdate_OPERATION_TOUCH,
			Relationship: &v1pb.Relationship{
				Resource: &v1pb.ObjectReference{
					ObjectType: model.AssessmentObject,
					ObjectId:   assessmentID,
				},
				Relation: model.EditorRelationshipKind.String(),
				Subject: &v1pb.SubjectReference{
					Object: &v1pb.ObjectReference{
						ObjectType: subject.Kind.String(),
						ObjectId:   subject.Id,
					},
				},
			},
		}

		if subject.Kind == model.Organization {
			relationshipUpdate.Relationship.Subject.OptionalRelation = model.MemberRelationshipKind.String()
		}

		return append(updates, relationshipUpdate)
	}
}

// WithMemberRelationship creates a relationship function that adds a user to an organization.
// This establishes a member relationship between the user and the organization, which may
// grant the user access to resources shared with the organization.
//
// Parameters:
//   - userID: The ID of the user to add to the organization
//   - orgID: The ID of the organization to add the user to
//
// Returns:
//   - model.RelationshipFn: A function that can be used with WriteRelationships or DeleteRelationships
//
// Example:
//
//	membershipFn := WithMemberRelationship("user123", "org456")
//
//	err := authzService.WriteRelationships(ctx, membershipFn)
//	if err != nil {
//	    log.Printf("Failed to add user to organization: %v", err)
//	}
//
//	// Can also be combined with other relationship operations:
//	userSubject := model.NewUserSubject("user123")
//	err = authzService.WriteRelationships(ctx,
//	    WithMemberRelationship("user123", "org456"),
//	    WithOwnerRelationship("assessment789", userSubject),
//	)
func WithMemberRelationship(userID, orgID string) model.RelationshipFn {
	return func(updates []*v1pb.RelationshipUpdate) []*v1pb.RelationshipUpdate {
		relationshipUpdate := &v1pb.RelationshipUpdate{
			Operation: v1pb.RelationshipUpdate_OPERATION_TOUCH,
			Relationship: &v1pb.Relationship{
				Resource: &v1pb.ObjectReference{
					ObjectType: model.OrgObject,
					ObjectId:   orgID,
				},
				Relation: model.MemberRelationshipKind.String(),
				Subject: &v1pb.SubjectReference{
					Object: &v1pb.ObjectReference{
						ObjectType: model.UserObject,
						ObjectId:   userID,
					},
				},
			},
		}

		return append(updates, relationshipUpdate)
	}
}
