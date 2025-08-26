package store

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"math/rand"

	v1pb "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/authzed-go/v1"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"go.uber.org/zap"
)

type ZedTokenKey struct{}

type Authz interface {
	WriteRelationships(ctx context.Context, relationships ...model.RelationshipFn) error
	DeleteRelationships(ctx context.Context, assessmentId string) error
	ListResources(ctx context.Context, userID string) ([]model.Resource, error)
	GetPermissions(ctx context.Context, assessmentIDs []string, userID string) (map[string][]model.Permission, error)
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
	callerID := fmt.Sprintf("write-%d", rand.Int31())
	zap.S().Debugw("WriteRelationships: acquiring exclusive lock", "callerID", callerID, "relationshipCount", len(relationships))
	if err := a.zedStore.AcquireLock(ctx, false); err != nil {
		zap.S().Errorw("WriteRelationships: failed to acquire exclusive lock", "callerID", callerID, "error", err)
		return err
	}

	zap.S().Debugw("WriteRelationships: exclusive lock acquired", "callerID", callerID)
	defer func() {
		zap.S().Debugw("WriteRelationships: releasing exclusive lock", "callerID", callerID)
		if err := a.zedStore.ReleaseLock(ctx, false); err != nil {
			zap.S().Warnw("WriteRelationships: failed to release exclusive lock", "callerID", callerID, "error", err)
		}
		zap.S().Debugw("WriteRelationships: lock released", "callerID", callerID)
	}()

	zap.S().Debugw("WriteRelationships: executing relationship writes", "callerID", callerID)
	zedToken, err := a.writeRelationships(ctx, relationships...)
	if err != nil {
		zap.S().Errorw("WriteRelationships: failed to write relationships", "error", err)
		return err
	}

	zap.S().Debugw("WriteRelationships: writing zed token to store", "token", zedToken.Token, "callerID", callerID)
	return a.zedStore.Write(ctx, zedToken.Token)
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
func (a *AuthzStore) DeleteRelationships(ctx context.Context, assessmentId string) error {
	callerID := fmt.Sprintf("delete-%d", rand.Int31())
	zap.S().Debugw("DeleteRelationships: acquiring exclusive lock", "callerID", callerID)
	if err := a.zedStore.AcquireLock(ctx, false); err != nil {
		zap.S().Errorw("DeleteRelationships: failed to acquire exclusive lock", "callerID", callerID, "error", err)
		return err
	}
	zap.S().Debugw("DeleteRelationships: exclusive lock acquired", "callerID", callerID)
	defer func() {
		zap.S().Debugw("DeleteRelationships: releasing exclusive lock", "callerID", callerID)
		if err := a.zedStore.ReleaseLock(ctx, false); err != nil {
			zap.S().Warnw("DeleteRelationships: failed to release exclusive lock", "callerID", callerID, "error", err)
		}
	}()

	zap.S().Debugw("DeleteRelationships: executing relationship deletions")
	zedToken, err := a.deleteRelationships(ctx, assessmentId)
	if err != nil {
		zap.S().Errorw("DeleteRelationships: failed to delete relationships", "error", err)
		return err
	}

	zap.S().Debugw("DeleteRelationships: writing zed token to store", "token", zedToken.Token)
	return a.zedStore.Write(ctx, zedToken.Token)
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
	// zap.S().Debugw("ListResources: acquiring shared lock", "userID", userID)
	// if err := a.zedStore.AcquireLock(ctx, true); err != nil {
	// 	zap.S().Errorw("ListResources: failed to acquire shared lock", "error", err, "userID", userID)
	// 	return []model.Resource{}, err
	// }
	// zap.S().Debugw("ListResources: shared lock acquired")
	// defer func() {
	// 	zap.S().Debugw("ListResources: releasing shared lock")
	// 	if err := a.zedStore.ReleaseLock(ctx, true); err != nil {
	// 		zap.S().Warnw("ListResources: failed to release shared lock", "error", err)
	// 	}
	// }()

	zap.S().Debugw("ListResources: reading zed token from store")
	token, err := a.zedStore.Read(ctx)
	if err != nil {
		zap.S().Errorw("ListResources: failed to read zed token", "error", err)
		return []model.Resource{}, err
	}

	zap.S().Debugw("ListResources: executing resource lookup", "hashedUserID", hash(userID))
	return a.listResources(a.TokenToContext(ctx, token), hash(userID))
}

// GetPermissions returns a map of permissions that a user has on multiple assessments.
// This method checks all possible permissions for each assessment and returns only those that the user actually has.
//
// Parameters:
//   - ctx: Context for the request
//   - assessmentIDs: A slice of assessment IDs to check permissions for
//   - userID: The ID of the user to check permissions for
//
// Returns:
//   - map[string][]model.Permission: A map where keys are assessment IDs and values are slices of permissions
//   - error: Error if the operation fails, nil on success
//
// Example:
//
//	assessmentIDs := []string{"assessment789", "assessment456"}
//	permissionsMap, err := authzService.GetPermissions(ctx, assessmentIDs, "user123")
//	if err != nil {
//	    log.Printf("Failed to get permissions: %v", err)
//	    return
//	}
//
//	for assessmentID, permissions := range permissionsMap {
//	    fmt.Printf("Assessment %s permissions: ", assessmentID)
//	    for _, perm := range permissions {
//	        fmt.Printf("%s ", perm.String())
//	    }
//	    fmt.Println()
//	}
func (a *AuthzStore) GetPermissions(ctx context.Context, assessmentIDs []string, userID string) (map[string][]model.Permission, error) {
	// zap.S().Debugw("GetPermissions: acquiring shared lock", "userID", userID, "assessmentCount", len(assessmentIDs))
	// if err := a.zedStore.AcquireLock(ctx, true); err != nil {
	// 	zap.S().Errorw("GetPermissions: failed to acquire shared lock", "error", err, "userID", userID)
	// 	return map[string][]model.Permission{}, err
	// }
	// zap.S().Debugw("GetPermissions: shared lock acquired")
	// defer func() {
	// 	zap.S().Debugw("GetPermissions: releasing shared lock")
	// 	if err := a.zedStore.ReleaseLock(ctx, true); err != nil {
	// 		zap.S().Warnw("GetPermissions: failed to release shared lock", "error", err)
	// 	}
	// }()

	// read the token first
	zap.S().Debugw("GetPermissions: reading zed token from store")
	token, err := a.zedStore.Read(ctx)
	if err != nil {
		zap.S().Errorw("GetPermissions: failed to read zed token", "error", err)
		return map[string][]model.Permission{}, err
	}

	zap.S().Debugw("GetPermissions: executing bulk permission checks", "hashedUserID", hash(userID), "assessmentIDs", assessmentIDs)
	return a.getBulkPermissions(a.TokenToContext(ctx, token), hash(userID), assessmentIDs)
}

func (a *AuthzStore) TokenToContext(ctx context.Context, token *string) context.Context {
	if token == nil {
		return ctx
	}
	return context.WithValue(ctx, ZedTokenKey{}, &v1pb.ZedToken{Token: *token})
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
						ObjectId:   hash(subject.Id),
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
						ObjectId:   hash(subject.Id),
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
						ObjectId:   hash(subject.Id),
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
					ObjectId:   hash(orgID),
				},
				Relation: model.MemberRelationshipKind.String(),
				Subject: &v1pb.SubjectReference{
					Object: &v1pb.ObjectReference{
						ObjectType: model.UserObject,
						ObjectId:   hash(userID),
					},
				},
			},
		}

		return append(updates, relationshipUpdate)
	}
}

// Private methods

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

// deleteRelationships is the private implementation of DeleteRelationships
func (a *AuthzStore) deleteRelationships(ctx context.Context, assessmentId string) (*v1pb.ZedToken, error) {
	resp, err := a.client.DeleteRelationships(ctx, &v1pb.DeleteRelationshipsRequest{
		RelationshipFilter: &v1pb.RelationshipFilter{
			ResourceType:       model.AssessmentObject,
			OptionalResourceId: assessmentId,
		},
	})
	if err != nil {
		return nil, err
	}

	return resp.DeletedAt, nil
}

// listResources is the private implementation of ListResources
func (a *AuthzStore) listResources(ctx context.Context, hashedUserID string) ([]model.Resource, error) {
	// try to get zedToken from ctx if any
	token := a.getZedToken(ctx)

	// Lookup resources for which the user has at least read permission
	req := &v1pb.LookupResourcesRequest{
		ResourceObjectType: model.AssessmentObject,
		Permission:         model.ReadPermission.String(),
		Subject: &v1pb.SubjectReference{
			Object: &v1pb.ObjectReference{
				ObjectType: model.UserObject,
				ObjectId:   hashedUserID,
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
		permissions, err := a.getPermissions(ctx, hashedUserID, resource.ResourceObjectId)
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

// getPermissions checks all possible permissions for a user on an assessment using bulk check
func (a *AuthzStore) getPermissions(ctx context.Context, hashedUserID string, assessmentID string) ([]model.Permission, error) {
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
					ObjectId:   hashedUserID,
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

// getBulkPermissions checks all possible permissions for a user on multiple assessments using bulk check
func (a *AuthzStore) getBulkPermissions(ctx context.Context, hashedUserID string, assessmentIDs []string) (map[string][]model.Permission, error) {
	if len(assessmentIDs) == 0 {
		return make(map[string][]model.Permission), nil
	}

	token := a.getZedToken(ctx)

	allPermissions := []model.Permission{
		model.ReadPermission,
		model.EditPermission,
		model.SharePermission,
		model.DeletePermission,
	}

	// Build bulk permission check request for all assessment-permission combinations
	var items []*v1pb.CheckBulkPermissionsRequestItem
	var itemIndex []struct {
		assessmentID string
		permission   model.Permission
	}

	for _, assessmentID := range assessmentIDs {
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
						ObjectId:   hashedUserID,
					},
				},
			})
			itemIndex = append(itemIndex, struct {
				assessmentID string
				permission   model.Permission
			}{assessmentID, perm})
		}
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

	// Build result map
	result := make(map[string][]model.Permission)
	for i, pair := range resp.Pairs {
		assessmentID := itemIndex[i].assessmentID
		permission := itemIndex[i].permission

		// Initialize slice if not exists
		if _, exists := result[assessmentID]; !exists {
			result[assessmentID] = []model.Permission{}
		}

		// Check if the response is an item (not an error)
		if item := pair.GetItem(); item != nil {
			if item.Permissionship == v1pb.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION {
				result[assessmentID] = append(result[assessmentID], permission)
			}
		}
		// If there's an error for this permission check, we skip it
	}

	return result, nil
}

func hash(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return fmt.Sprintf("%x", h.Sum(nil))[:6]
}
