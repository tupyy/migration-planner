package store_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"time"

	v1pb "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/authzed-go/v1"
	"github.com/authzed/grpcutil"
	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gorm.io/gorm"
)

var _ = Describe("AuthzStore", Ordered, func() {
	var (
		authzSvc      *store.AuthzStore
		spiceDBClient *authzed.Client
		ctx           context.Context
		zedToken      *v1pb.ZedToken
		gormDB        *gorm.DB
		zedTokenStore *store.ZedTokenStore
	)

	BeforeAll(func() {
		ctx = context.Background()

		// Skip tests if SpiceDB is not available
		spiceDBEndpoint := os.Getenv("SPICEDB_ENDPOINT")
		if spiceDBEndpoint == "" {
			spiceDBEndpoint = "localhost:50051"
		}

		// Create SpiceDB client
		var err error
		spiceDBClient, err = authzed.NewClient(
			spiceDBEndpoint,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpcutil.WithInsecureBearerToken("foobar"),
		)
		if err != nil {
			zap.S().Error(err)
			Skip("SpiceDB not available: " + err.Error())
		}

		// Test connection
		_, err = spiceDBClient.ReadSchema(ctx, &v1pb.ReadSchemaRequest{})
		if err != nil {
			Skip("SpiceDB not reachable: " + err.Error())
		}

		// Initialize database using the same pattern as other tests
		cfg, err := config.New()
		Expect(err).To(BeNil())

		gormDB, err = store.InitDB(cfg)
		Expect(err).To(BeNil())

		zedTokenStore = store.NewZedTokenStore(gormDB)
		authzSvc = store.NewAuthzStore(zedTokenStore, spiceDBClient).(*store.AuthzStore)
	})

	AfterAll(func() {
		if spiceDBClient != nil {
			spiceDBClient.Close()
		}
	})

	Context("User-Organization Membership", func() {
		// Test: Validates that the authz service can establish membership relationships between users and organizations
		// Expected: User should be successfully added as a member of the organization, verifiable through direct SpiceDB queries
		// Purpose: Tests the fundamental prerequisite for all organization-based permissions
		It("should write user-organization membership relationship successfully", func() {
			userID := "user_" + uuid.New().String()[:8]
			orgID := "org_" + uuid.New().String()[:8]

			err := authzSvc.WriteRelationships(ctx, store.WithMemberRelationship(userID, orgID))
			Expect(err).To(BeNil())

			err = verifyUserOrganizationMembership(ctx, spiceDBClient, zedTokenStore, userID, orgID)
			Expect(err).To(BeNil())
		})
	})

	Context("User-Assessment relationships", func() {
		// Test: Validates that users can be granted direct owner permissions on assessments
		// Expected: Owner should have all permissions (read, edit, share, delete) on the assessment
		// Purpose: Tests the highest privilege level in the authorization model
		It("should write owner relationship and verify all permissions", func() {
			userID := "user_" + uuid.New().String()[:8]
			orgID := "org_" + uuid.New().String()[:8]
			assessmentID := "assessment_" + uuid.New().String()[:8]

			_, err := setupUserOrganizationMembership(ctx, spiceDBClient, zedTokenStore, userID, orgID)
			Expect(err).To(BeNil())

			subject := model.NewUserSubject(userID)
			err = authzSvc.WriteRelationships(ctx, store.WithOwnerRelationship(assessmentID, subject))
			Expect(err).To(BeNil())

			<-time.After(5000 * time.Microsecond)

			resp, err := spiceDBClient.ReadRelationships(ctx, &v1pb.ReadRelationshipsRequest{
				Consistency: &v1pb.Consistency{
					Requirement: &v1pb.Consistency_FullyConsistent{
						FullyConsistent: true,
					},
				},
				RelationshipFilter: &v1pb.RelationshipFilter{
					ResourceType:       model.AssessmentObject,
					OptionalResourceId: assessmentID,
					OptionalRelation:   "owner",
					OptionalSubjectFilter: &v1pb.SubjectFilter{
						SubjectType:       model.UserObject,
						OptionalSubjectId: hash(userID),
					},
				},
			})
			Expect(err).To(BeNil())

			relationships := []*v1pb.Relationship{}
			for {
				rel, err := resp.Recv()
				if err != nil {
					break
				}
				relationships = append(relationships, rel.Relationship)
			}
			Expect(relationships).To(HaveLen(1))
			Expect(relationships[0].Resource.ObjectType).To(Equal(model.AssessmentObject))
			Expect(relationships[0].Resource.ObjectId).To(Equal(assessmentID))
			Expect(relationships[0].Relation).To(Equal("owner"))
			Expect(relationships[0].Subject.Object.ObjectType).To(Equal(model.UserObject))
			Expect(relationships[0].Subject.Object.ObjectId).To(Equal(hash(userID)))
		})

		// Test: Validates that users can be granted direct editor permissions on assessments
		// Expected: Editor should have read and edit permissions but not share or delete permissions
		// Purpose: Tests the middle privilege level that allows content modification without ownership rights
		It("should write editor relationship and verify correct permissions", func() {
			userID := "user_" + uuid.New().String()[:8]
			orgID := "org_" + uuid.New().String()[:8]
			assessmentID := "assessment-editor-" + uuid.New().String()[:8]

			_, err := setupUserOrganizationMembership(ctx, spiceDBClient, zedTokenStore, userID, orgID)
			Expect(err).To(BeNil())

			subject := model.NewUserSubject(userID)
			err = authzSvc.WriteRelationships(ctx, store.WithEditorRelationship(assessmentID, subject))
			Expect(err).To(BeNil())

			resp, err := spiceDBClient.ReadRelationships(ctx, &v1pb.ReadRelationshipsRequest{
				Consistency: &v1pb.Consistency{
					Requirement: &v1pb.Consistency_FullyConsistent{
						FullyConsistent: true,
					},
				},
				RelationshipFilter: &v1pb.RelationshipFilter{
					ResourceType:       model.AssessmentObject,
					OptionalResourceId: assessmentID,
					OptionalRelation:   "editor",
					OptionalSubjectFilter: &v1pb.SubjectFilter{
						SubjectType:       model.UserObject,
						OptionalSubjectId: hash(userID),
					},
				},
			})
			Expect(err).To(BeNil())

			relationships := []*v1pb.Relationship{}
			for {
				rel, err := resp.Recv()
				if err != nil {
					break
				}
				relationships = append(relationships, rel.Relationship)
			}
			Expect(relationships).To(HaveLen(1))
			Expect(relationships[0].Resource.ObjectType).To(Equal(model.AssessmentObject))
			Expect(relationships[0].Resource.ObjectId).To(Equal(assessmentID))
			Expect(relationships[0].Relation).To(Equal("editor"))
			Expect(relationships[0].Subject.Object.ObjectType).To(Equal(model.UserObject))
			Expect(relationships[0].Subject.Object.ObjectId).To(Equal(hash(userID)))
		})

		// Test: Validates that users can be granted direct reader permissions on assessments
		// Expected: Reader should only have read permission, no write/edit/share/delete permissions
		// Purpose: Tests the lowest privilege level that provides read-only access
		It("should write reader relationship and verify correct permissions", func() {
			userID := "user_" + uuid.New().String()[:8]
			orgID := "org_" + uuid.New().String()[:8]
			assessmentID := "assessment-reader-" + uuid.New().String()[:8]

			_, err := setupUserOrganizationMembership(ctx, spiceDBClient, zedTokenStore, userID, orgID)
			Expect(err).To(BeNil())

			subject := model.NewUserSubject(userID)
			err = authzSvc.WriteRelationships(ctx, store.WithReaderRelationship(assessmentID, subject))
			Expect(err).To(BeNil())

			resp, err := spiceDBClient.ReadRelationships(ctx, &v1pb.ReadRelationshipsRequest{
				Consistency: &v1pb.Consistency{
					Requirement: &v1pb.Consistency_FullyConsistent{
						FullyConsistent: true,
					},
				},
				RelationshipFilter: &v1pb.RelationshipFilter{
					ResourceType:       model.AssessmentObject,
					OptionalResourceId: assessmentID,
					OptionalRelation:   "reader",
					OptionalSubjectFilter: &v1pb.SubjectFilter{
						SubjectType:       model.UserObject,
						OptionalSubjectId: hash(userID),
					},
				},
			})
			Expect(err).To(BeNil())

			relationships := []*v1pb.Relationship{}
			for {
				rel, err := resp.Recv()
				if err != nil {
					break
				}
				relationships = append(relationships, rel.Relationship)
			}
			Expect(relationships).To(HaveLen(1))
			Expect(relationships[0].Resource.ObjectType).To(Equal(model.AssessmentObject))
			Expect(relationships[0].Resource.ObjectId).To(Equal(assessmentID))
			Expect(relationships[0].Relation).To(Equal("reader"))
			Expect(relationships[0].Subject.Object.ObjectType).To(Equal(model.UserObject))
			Expect(relationships[0].Subject.Object.ObjectId).To(Equal(hash(userID)))
		})
	})

	Context("Organization-Assessment relationships", func() {
		// Test: Validates that organizations can be granted owner permissions on assessments
		// Expected: Organization should be recorded as owner, enabling all members to inherit owner permissions
		// Purpose: Tests organization-level ownership that cascades to all organization members
		It("should write organization owner relationship", func() {
			// Create local test data
			userID := "user_" + uuid.New().String()[:8]
			orgID := "org_" + uuid.New().String()[:8]
			assessmentID := "assessment-org-owner-" + uuid.New().String()[:8]

			// Setup user membership in organization (prerequisite)
			_, err := setupUserOrganizationMembership(ctx, spiceDBClient, zedTokenStore, userID, orgID)
			Expect(err).To(BeNil())

			subject := model.NewOrganizationSubject(orgID)
			err = authzSvc.WriteRelationships(ctx, store.WithOwnerRelationship(assessmentID, subject))
			Expect(err).To(BeNil())

			// Verify the relationship was written by reading it back with full consistency
			resp, err := spiceDBClient.ReadRelationships(ctx, &v1pb.ReadRelationshipsRequest{
				Consistency: &v1pb.Consistency{
					Requirement: &v1pb.Consistency_FullyConsistent{
						FullyConsistent: true,
					},
				},
				RelationshipFilter: &v1pb.RelationshipFilter{
					ResourceType:       model.AssessmentObject,
					OptionalResourceId: assessmentID,
					OptionalRelation:   "owner",
					OptionalSubjectFilter: &v1pb.SubjectFilter{
						SubjectType:       model.OrgObject,
						OptionalSubjectId: hash(orgID),
					},
				},
			})
			Expect(err).To(BeNil())

			relationships := []*v1pb.Relationship{}
			for {
				rel, err := resp.Recv()
				if err != nil {
					break
				}
				relationships = append(relationships, rel.Relationship)
			}
			Expect(relationships).To(HaveLen(1))
			Expect(relationships[0].Resource.ObjectType).To(Equal(model.AssessmentObject))
			Expect(relationships[0].Resource.ObjectId).To(Equal(assessmentID))
			Expect(relationships[0].Relation).To(Equal("owner"))
			Expect(relationships[0].Subject.Object.ObjectType).To(Equal(model.OrgObject))
			Expect(relationships[0].Subject.Object.ObjectId).To(Equal(hash(orgID)))
		})

		// Test: Validates that organizations can be granted editor permissions on assessments
		// Expected: Organization should be recorded as editor, enabling members to inherit editor permissions
		// Purpose: Tests organization-level editing rights that cascade to organization members
		It("should write organization editor relationship", func() {
			// Create local test data
			userID := "user_" + uuid.New().String()[:8]
			orgID := "org_" + uuid.New().String()[:8]
			assessmentID := "assessment-org-editor-" + uuid.New().String()[:8]

			// Setup user membership in organization (prerequisite)
			_, err := setupUserOrganizationMembership(ctx, spiceDBClient, zedTokenStore, userID, orgID)
			Expect(err).To(BeNil())

			subject := model.NewOrganizationSubject(orgID)
			err = authzSvc.WriteRelationships(ctx, store.WithEditorRelationship(assessmentID, subject))
			Expect(err).To(BeNil())

			// Verify the relationship was written by reading it back with full consistency
			resp, err := spiceDBClient.ReadRelationships(ctx, &v1pb.ReadRelationshipsRequest{
				Consistency: &v1pb.Consistency{
					Requirement: &v1pb.Consistency_FullyConsistent{
						FullyConsistent: true,
					},
				},
				RelationshipFilter: &v1pb.RelationshipFilter{
					ResourceType:       model.AssessmentObject,
					OptionalResourceId: assessmentID,
					OptionalRelation:   "editor",
					OptionalSubjectFilter: &v1pb.SubjectFilter{
						SubjectType:       model.OrgObject,
						OptionalSubjectId: hash(orgID),
					},
				},
			})
			Expect(err).To(BeNil())

			relationships := []*v1pb.Relationship{}
			for {
				rel, err := resp.Recv()
				if err != nil {
					break
				}
				relationships = append(relationships, rel.Relationship)
			}
			Expect(relationships).To(HaveLen(1))
			Expect(relationships[0].Resource.ObjectType).To(Equal(model.AssessmentObject))
			Expect(relationships[0].Resource.ObjectId).To(Equal(assessmentID))
			Expect(relationships[0].Relation).To(Equal("editor"))
			Expect(relationships[0].Subject.Object.ObjectType).To(Equal(model.OrgObject))
			Expect(relationships[0].Subject.Object.ObjectId).To(Equal(hash(orgID)))
		})

		// Test: Validates that organizations can be granted reader permissions on assessments
		// Expected: Organization should be recorded as reader, enabling members to inherit read permissions
		// Purpose: Tests organization-level read access that cascades to organization members
		It("should write organization reader relationship", func() {
			// Create local test data
			userID := "user_" + uuid.New().String()[:8]
			orgID := "org_" + uuid.New().String()[:8]
			assessmentID := "assessment-org-reader-" + uuid.New().String()[:8]

			// Setup user membership in organization (prerequisite)
			_, err := setupUserOrganizationMembership(ctx, spiceDBClient, zedTokenStore, userID, orgID)
			Expect(err).To(BeNil())

			subject := model.NewOrganizationSubject(orgID)
			err = authzSvc.WriteRelationships(ctx, store.WithReaderRelationship(assessmentID, subject))
			Expect(err).To(BeNil())

			// Verify the relationship was written by reading it back with full consistency
			resp, err := spiceDBClient.ReadRelationships(ctx, &v1pb.ReadRelationshipsRequest{
				Consistency: &v1pb.Consistency{
					Requirement: &v1pb.Consistency_FullyConsistent{
						FullyConsistent: true,
					},
				},
				RelationshipFilter: &v1pb.RelationshipFilter{
					ResourceType:       model.AssessmentObject,
					OptionalResourceId: assessmentID,
					OptionalRelation:   "reader",
					OptionalSubjectFilter: &v1pb.SubjectFilter{
						SubjectType:       model.OrgObject,
						OptionalSubjectId: hash(orgID),
					},
				},
			})
			Expect(err).To(BeNil())

			relationships := []*v1pb.Relationship{}
			for {
				rel, err := resp.Recv()
				if err != nil {
					break
				}
				relationships = append(relationships, rel.Relationship)
			}
			Expect(relationships).To(HaveLen(1))
			Expect(relationships[0].Resource.ObjectType).To(Equal(model.AssessmentObject))
			Expect(relationships[0].Resource.ObjectId).To(Equal(assessmentID))
			Expect(relationships[0].Relation).To(Equal("reader"))
			Expect(relationships[0].Subject.Object.ObjectType).To(Equal(model.OrgObject))
			Expect(relationships[0].Subject.Object.ObjectId).To(Equal(hash(orgID)))
		})
	})

	Context("List Permissions", func() {
		// Test: Validates that the ListResources method correctly discovers and returns user permissions across multiple assessments
		// Expected: Should return 3 assessments with correct permission sets:
		//   - Assessment1: All permissions (owner through direct user relationship)
		//   - Assessment2: Read-only (reader through direct user relationship)
		//   - Assessment3: Read+Edit (editor through organization membership)
		// Purpose: Tests complex permission aggregation from both direct user relationships and organizational membership
		It("should return correct permissions for user with different access patterns", func() {
			userID := "list-user-" + uuid.New().String()[:8]
			orgID := "list-org-" + uuid.New().String()[:8]
			assessmentID1 := "list-assessment1-" + uuid.New().String()[:8]
			assessmentID2 := "list-assessment2-" + uuid.New().String()[:8]
			assessmentID3 := "list-assessment3-" + uuid.New().String()[:8]
			resp1, err := spiceDBClient.WriteRelationships(ctx, &v1pb.WriteRelationshipsRequest{
				Updates: []*v1pb.RelationshipUpdate{
					{
						Operation: v1pb.RelationshipUpdate_OPERATION_CREATE,
						Relationship: &v1pb.Relationship{
							Resource: &v1pb.ObjectReference{
								ObjectType: model.OrgObject,
								ObjectId:   hash(orgID),
							},
							Relation: "member",
							Subject: &v1pb.SubjectReference{
								Object: &v1pb.ObjectReference{
									ObjectType: model.UserObject,
									ObjectId:   hash(userID),
								},
							},
						},
					},
				},
			})
			Expect(err).To(BeNil())

			err = zedTokenStore.Write(ctx, resp1.WrittenAt.Token)
			Expect(err).To(BeNil())

			updates := []*v1pb.RelationshipUpdate{
				{
					Operation: v1pb.RelationshipUpdate_OPERATION_CREATE,
					Relationship: &v1pb.Relationship{
						Resource: &v1pb.ObjectReference{
							ObjectType: model.AssessmentObject,
							ObjectId:   assessmentID1,
						},
						Relation: "owner",
						Subject: &v1pb.SubjectReference{
							Object: &v1pb.ObjectReference{
								ObjectType: model.UserObject,
								ObjectId:   hash(userID),
							},
						},
					},
				},
				{
					Operation: v1pb.RelationshipUpdate_OPERATION_CREATE,
					Relationship: &v1pb.Relationship{
						Resource: &v1pb.ObjectReference{
							ObjectType: model.AssessmentObject,
							ObjectId:   assessmentID2,
						},
						Relation: "reader",
						Subject: &v1pb.SubjectReference{
							Object: &v1pb.ObjectReference{
								ObjectType: model.UserObject,
								ObjectId:   hash(userID),
							},
						},
					},
				},
				{
					Operation: v1pb.RelationshipUpdate_OPERATION_CREATE,
					Relationship: &v1pb.Relationship{
						Resource: &v1pb.ObjectReference{
							ObjectType: model.AssessmentObject,
							ObjectId:   assessmentID3,
						},
						Relation: "editor",
						Subject: &v1pb.SubjectReference{
							Object: &v1pb.ObjectReference{
								ObjectType: model.OrgObject,
								ObjectId:   hash(orgID),
							},
							OptionalRelation: "member",
						},
					},
				},
			}

			token, err := spiceDBClient.WriteRelationships(ctx, &v1pb.WriteRelationshipsRequest{
				Updates: updates,
			})
			Expect(err).To(BeNil())

			err = zedTokenStore.Write(ctx, token.WrittenAt.Token)
			Expect(err).To(BeNil())
			resources, err := authzSvc.ListResources(ctx, userID)
			Expect(err).To(BeNil())
			Expect(resources).To(HaveLen(3), "User should have access to 3 assessments")

			for _, resource := range resources {
				switch resource.AssessmentID {
				case assessmentID1:
					Expect(resource.Permissions).To(ContainElements(
						model.ReadPermission,
						model.EditPermission,
						model.SharePermission,
						model.DeletePermission,
					), "Assessment1: Owner should have all permissions")

				case assessmentID2:
					Expect(resource.Permissions).To(ContainElement(model.ReadPermission),
						"Assessment2: Reader should have read permission")
					Expect(resource.Permissions).ToNot(ContainElements(
						model.EditPermission,
						model.SharePermission,
						model.DeletePermission,
					), "Assessment2: Reader should not have write permissions")

				case assessmentID3:
					Expect(resource.Permissions).To(ContainElements(
						model.ReadPermission,
						model.EditPermission,
					), "Assessment3: User should have read and edit permissions through org editor relationship")
					Expect(resource.Permissions).ToNot(ContainElements(
						model.SharePermission,
						model.DeletePermission,
					), "Assessment3: User should not have owner-only permissions through org editor relationship")
				}
			}
		})

		// Test: Validates that users without any permissions receive an empty resource list
		// Expected: Should return empty list for user who is not a member of any organization and has no direct permissions
		// Purpose: Tests proper access control isolation - users should only see resources they have access to
		It("should return empty list for user with no permissions", func() {
			nonMemberUserID := "no-access-user-" + uuid.New().String()[:8]
			resources, err := authzSvc.ListResources(ctx, nonMemberUserID)
			Expect(err).To(BeNil())
			Expect(resources).To(HaveLen(0), "User with no permissions should get empty list")
		})
	})

	Context("Delete Relationships", func() {
		// Context: Tests the deletion of authorization relationships and proper cleanup
		// Purpose: Validates that relationships can be completely removed from SpiceDB and tokens remain consistent
		var (
			deleteTestUserID        string
			deleteTestAssessmentID1 string
			deleteTestAssessmentID2 string
			testOrgID               string
		)

		BeforeEach(func() {
			deleteTestUserID = "delete-user-" + uuid.New().String()[:8]
			deleteTestAssessmentID1 = "delete-assessment1-" + uuid.New().String()[:8]
			deleteTestAssessmentID2 = "delete-assessment2-" + uuid.New().String()[:8]
			testOrgID = "org-" + uuid.New().String()[:8]
			_, err := setupUserOrganizationMembership(ctx, spiceDBClient, zedTokenStore, deleteTestUserID, testOrgID)
			Expect(err).To(BeNil())

			resp, err := spiceDBClient.WriteRelationships(ctx, &v1pb.WriteRelationshipsRequest{
				Updates: []*v1pb.RelationshipUpdate{
					{
						Operation: v1pb.RelationshipUpdate_OPERATION_CREATE,
						Relationship: &v1pb.Relationship{
							Resource: &v1pb.ObjectReference{
								ObjectType: model.AssessmentObject,
								ObjectId:   deleteTestAssessmentID1,
							},
							Relation: "editor",
							Subject: &v1pb.SubjectReference{
								Object: &v1pb.ObjectReference{
									ObjectType: model.UserObject,
									ObjectId:   hash(deleteTestUserID),
								},
							},
						},
					},
					{
						Operation: v1pb.RelationshipUpdate_OPERATION_CREATE,
						Relationship: &v1pb.Relationship{
							Resource: &v1pb.ObjectReference{
								ObjectType: model.AssessmentObject,
								ObjectId:   deleteTestAssessmentID2,
							},
							Relation: "reader",
							Subject: &v1pb.SubjectReference{
								Object: &v1pb.ObjectReference{
									ObjectType: model.UserObject,
									ObjectId:   hash(deleteTestUserID),
								},
							},
						},
					},
					{
						Operation: v1pb.RelationshipUpdate_OPERATION_CREATE,
						Relationship: &v1pb.Relationship{
							Resource: &v1pb.ObjectReference{
								ObjectType: model.AssessmentObject,
								ObjectId:   deleteTestAssessmentID2,
							},
							Relation: "editor",
							Subject: &v1pb.SubjectReference{
								Object: &v1pb.ObjectReference{
									ObjectType: model.UserObject,
									ObjectId:   hash(deleteTestUserID),
								},
							},
						},
					},
				},
			})
			Expect(err).To(BeNil())
			zedToken = resp.WrittenAt

			err = zedTokenStore.Write(ctx, zedToken.Token)
			Expect(err).To(BeNil())
		})

		// Test: Validates that individual assessment relationships can be completely deleted
		// Expected: All relationships for the assessment should be removed, user should lose all permissions
		// Purpose: Tests proper cleanup functionality and verification through direct SpiceDB queries
		It("should delete a relationship successfully", func() {
			tokenStr, err := zedTokenStore.Read(ctx)
			Expect(err).To(BeNil())
			token := &v1pb.ZedToken{Token: *tokenStr}

			checkResp, err := spiceDBClient.CheckPermission(ctx, &v1pb.CheckPermissionRequest{
				Consistency: &v1pb.Consistency{
					Requirement: &v1pb.Consistency_AtLeastAsFresh{
						AtLeastAsFresh: token,
					},
				},
				Resource: &v1pb.ObjectReference{
					ObjectType: model.AssessmentObject,
					ObjectId:   deleteTestAssessmentID1,
				},
				Permission: model.EditPermission.String(),
				Subject: &v1pb.SubjectReference{
					Object: &v1pb.ObjectReference{
						ObjectType: model.UserObject,
						ObjectId:   hash(deleteTestUserID),
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(checkResp.Permissionship).To(Equal(v1pb.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION), "Editor relationship should exist before deletion")

			err = authzSvc.DeleteRelationships(ctx, deleteTestAssessmentID1)
			Expect(err).To(BeNil())

			currentTokenStr, err := zedTokenStore.Read(ctx)
			Expect(err).To(BeNil())
			currentToken := &v1pb.ZedToken{Token: *currentTokenStr}

			checkResp, err = spiceDBClient.CheckPermission(ctx, &v1pb.CheckPermissionRequest{
				Resource: &v1pb.ObjectReference{
					ObjectType: model.AssessmentObject,
					ObjectId:   deleteTestAssessmentID1,
				},
				Permission: model.EditPermission.String(),
				Subject: &v1pb.SubjectReference{
					Object: &v1pb.ObjectReference{
						ObjectType: model.UserObject,
						ObjectId:   hash(deleteTestUserID),
					},
				},
				Consistency: &v1pb.Consistency{
					Requirement: &v1pb.Consistency_AtLeastAsFresh{
						AtLeastAsFresh: currentToken,
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(checkResp.Permissionship).To(Equal(v1pb.CheckPermissionResponse_PERMISSIONSHIP_NO_PERMISSION), "Editor relationship should be deleted")

			resp, err := spiceDBClient.ReadRelationships(ctx, &v1pb.ReadRelationshipsRequest{
				RelationshipFilter: &v1pb.RelationshipFilter{
					ResourceType:       model.AssessmentObject,
					OptionalResourceId: deleteTestAssessmentID1,
					OptionalRelation:   "editor",
					OptionalSubjectFilter: &v1pb.SubjectFilter{
						SubjectType:       model.UserObject,
						OptionalSubjectId: hash(deleteTestUserID),
					},
				},
			})
			Expect(err).To(BeNil())

			relationships := []*v1pb.Relationship{}
			for {
				rel, err := resp.Recv()
				if err != nil {
					break
				}
				relationships = append(relationships, rel.Relationship)
			}
			Expect(relationships).To(HaveLen(0), "Editor relationship should be completely removed")
		})

		// Test: Validates that ZedToken consistency is maintained across multiple sequential authz service operations
		// Expected: Write→Read→Delete→Read sequence should maintain proper token consistency in database
		// Purpose: Tests the internal token management system ensuring read-after-write consistency
		It("should maintain consistency token across chained service operations", func() {
			chainTestUserID := "chain-user-" + uuid.New().String()[:8]
			chainTestOrgID := "chain-org-" + uuid.New().String()[:8]
			chainTestAssessmentID := "chain-assessment-" + uuid.New().String()[:8]

			_, err := setupUserOrganizationMembership(ctx, spiceDBClient, zedTokenStore, chainTestUserID, chainTestOrgID)
			Expect(err).To(BeNil())

			subject := model.NewUserSubject(chainTestUserID)
			err = authzSvc.WriteRelationships(ctx, store.WithOwnerRelationship(chainTestAssessmentID, subject))
			Expect(err).To(BeNil())

			storedToken, err := zedTokenStore.Read(ctx)
			Expect(err).To(BeNil())
			Expect(storedToken).ToNot(BeNil(), "Service should have written ZedToken to database from write operation")

			permissionsMap, err := authzSvc.GetPermissions(ctx, []string{chainTestAssessmentID}, chainTestUserID)
			Expect(err).To(BeNil())
			Expect(permissionsMap).To(HaveKey(chainTestAssessmentID))
			permissions := permissionsMap[chainTestAssessmentID]
			Expect(permissions).To(ContainElement(model.ReadPermission), "Owner should have read permission")
			Expect(permissions).To(ContainElement(model.EditPermission), "Owner should have edit permission")
			Expect(permissions).To(ContainElement(model.SharePermission), "Owner should have share permission")
			Expect(permissions).To(ContainElement(model.DeletePermission), "Owner should have delete permission")

			err = authzSvc.DeleteRelationships(ctx, chainTestAssessmentID)
			Expect(err).To(BeNil())

			updatedToken, err := zedTokenStore.Read(ctx)
			Expect(err).To(BeNil())
			Expect(updatedToken).ToNot(BeNil(), "Service should have updated ZedToken in database from delete operation")
			Expect(*updatedToken).ToNot(Equal(*storedToken), "Token should be different after delete operation")

			permissionsMap, err = authzSvc.GetPermissions(ctx, []string{chainTestAssessmentID}, chainTestUserID)
			Expect(err).To(BeNil())
			if permissions, exists := permissionsMap[chainTestAssessmentID]; exists {
				Expect(permissions).To(BeEmpty(), "All permissions should be gone after delete")
			}

		})
	})
})

// Helper function to setup user membership in organization
// Purpose: Creates the prerequisite membership relationship that enables organization-based permissions
// Returns: ZedToken from the write operation for consistency tracking
func setupUserOrganizationMembership(ctx context.Context, client *authzed.Client, zedStore *store.ZedTokenStore, userID, orgID string) (*v1pb.ZedToken, error) {
	resp, err := client.WriteRelationships(ctx, &v1pb.WriteRelationshipsRequest{
		Updates: []*v1pb.RelationshipUpdate{
			{
				Operation: v1pb.RelationshipUpdate_OPERATION_TOUCH,
				Relationship: &v1pb.Relationship{
					Resource: &v1pb.ObjectReference{
						ObjectType: model.OrgObject,
						ObjectId:   hash(orgID),
					},
					Relation: "member",
					Subject: &v1pb.SubjectReference{
						Object: &v1pb.ObjectReference{
							ObjectType: model.UserObject,
							ObjectId:   hash(userID),
						},
					},
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	if err := zedStore.Write(ctx, resp.WrittenAt.Token); err != nil {
		return nil, fmt.Errorf("failed to write token to database: %w", err)
	}

	return resp.WrittenAt, nil
}

// Helper function to verify user membership in organization
// Purpose: Validates that a user is properly registered as a member of an organization
// Uses: Direct SpiceDB client to verify the relationship exists with proper consistency
func verifyUserOrganizationMembership(ctx context.Context, client *authzed.Client, zedStore *store.ZedTokenStore, userID, orgID string) error {
	tokenStr, err := zedStore.Read(ctx)
	if err != nil {
		return fmt.Errorf("failed to read token: %w", err)
	}

	var consistency *v1pb.Consistency
	if tokenStr != nil && *tokenStr != "" {
		consistency = &v1pb.Consistency{
			Requirement: &v1pb.Consistency_AtLeastAsFresh{
				AtLeastAsFresh: &v1pb.ZedToken{Token: *tokenStr},
			},
		}
	} else {
		consistency = &v1pb.Consistency{
			Requirement: &v1pb.Consistency_FullyConsistent{
				FullyConsistent: true,
			},
		}
	}

	resp, err := client.ReadRelationships(ctx, &v1pb.ReadRelationshipsRequest{
		Consistency: consistency,
		RelationshipFilter: &v1pb.RelationshipFilter{
			ResourceType:       model.OrgObject,
			OptionalResourceId: hash(orgID),
			OptionalRelation:   "member",
			OptionalSubjectFilter: &v1pb.SubjectFilter{
				SubjectType:       model.UserObject,
				OptionalSubjectId: hash(userID),
			},
		},
	})
	if err != nil {
		return err
	}

	relationships := []*v1pb.Relationship{}
	for {
		rel, err := resp.Recv()
		if err != nil {
			break
		}
		relationships = append(relationships, rel.Relationship)
	}

	if len(relationships) == 0 {
		return fmt.Errorf("user %s is not a member of organization %s", userID, orgID)
	}
	return nil
}

// hash function matching the one in authz.go
// Purpose: Creates consistent 6-character SHA256 hashes for user/org IDs as used by the authz service
// Note: All direct SpiceDB operations must use hashed IDs while service calls use original IDs
func hash(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return fmt.Sprintf("%x", h.Sum(nil))[:6]
}
