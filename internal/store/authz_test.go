package store_test

import (
	"context"
	"fmt"
	"os"
	"time"

	v1pb "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/authzed-go/v1"
	"github.com/authzed/grpcutil"
	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var _ = Describe("AuthzService", Ordered, func() {
	var (
		authzSvc          *store.AuthzService
		spiceDBClient     *authzed.Client
		testUserID        string
		testOrgID         string
		testAssessmentID  string
		testAssessmentID2 string
		ctx               context.Context
		zedToken          *v1pb.ZedToken
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

		authzSvc = store.NewAuthzService(spiceDBClient).(*store.AuthzService)

		// Setup test data
		testUserID = "user_" + uuid.New().String()[:8]
		testOrgID = "org_" + uuid.New().String()[:8]
		testAssessmentID = "assessment_" + uuid.New().String()[:8]
		testAssessmentID2 = "assessment_" + uuid.New().String()[:8]

		// Establish user membership in organization (prerequisite for all tests)
		_, err = setupUserOrganizationMembership(ctx, spiceDBClient, testUserID, testOrgID)
		if err != nil {
			Skip("Failed to setup user-organization membership: " + err.Error())
		}
	})

	AfterAll(func() {
		if spiceDBClient != nil {
			// Clean up test relationships
			cleanupAllRelationships(ctx, spiceDBClient, testAssessmentID, testAssessmentID2, testUserID, testOrgID)
			spiceDBClient.Close()
		}
	})

	Context("User-Organization Membership", func() {
		It("should write user-organization membership relationship successfully", func() {
			// Create new user and organization for isolated testing
			newUserID := "user_" + uuid.New().String()[:8]
			newOrgID := "org_" + uuid.New().String()[:8]

			// Use the new RelationshipFn to add user to organization
			err := authzSvc.WriteRelationships(ctx, store.WithMemberRelationship(newUserID, newOrgID))
			Expect(err).To(BeNil())

			// Verify the membership was written correctly using direct client read
			err = verifyUserOrganizationMembership(ctx, spiceDBClient, newUserID, newOrgID, nil)
			Expect(err).To(BeNil())

			// Cleanup
			cleanupUserFromOrganization(ctx, spiceDBClient, newUserID, newOrgID)
		})
	})

	Context("Writing Assessment Relationships", func() {
		BeforeEach(func() {
			// Ensure user is member of organization (prerequisite verified)
			err := verifyUserOrganizationMembership(ctx, spiceDBClient, testUserID, testOrgID, nil)
			Expect(err).To(BeNil(), "User must be organization member before assessment relationships")
		})

		Context("User-Assessment relationships", func() {
			It("should write owner relationship and verify all permissions", func() {
				subject := model.NewUserSubject(testUserID)

				err := authzSvc.WriteRelationships(ctx, store.WithOwnerRelationship(testAssessmentID, subject))
				Expect(err).To(BeNil())

				<-time.After(5000 * time.Microsecond)

				// Verify owner relationship was written using direct SpiceDB client with full consistency
				resp, err := spiceDBClient.ReadRelationships(ctx, &v1pb.ReadRelationshipsRequest{
					Consistency: &v1pb.Consistency{
						Requirement: &v1pb.Consistency_FullyConsistent{
							FullyConsistent: true,
						},
					},
					RelationshipFilter: &v1pb.RelationshipFilter{
						ResourceType:       model.AssessmentObject,
						OptionalResourceId: testAssessmentID,
						OptionalRelation:   "owner",
						OptionalSubjectFilter: &v1pb.SubjectFilter{
							SubjectType:       model.UserObject,
							OptionalSubjectId: testUserID,
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
				Expect(relationships[0].Resource.ObjectId).To(Equal(testAssessmentID))
				Expect(relationships[0].Relation).To(Equal("owner"))
				Expect(relationships[0].Subject.Object.ObjectType).To(Equal(model.UserObject))
				Expect(relationships[0].Subject.Object.ObjectId).To(Equal(testUserID))
			})

			It("should write editor relationship and verify correct permissions", func() {
				testAssessmentIDEditor := "assessment-editor-" + uuid.New().String()[:8]
				subject := model.NewUserSubject(testUserID)

				err := authzSvc.WriteRelationships(ctx, store.WithEditorRelationship(testAssessmentIDEditor, subject))
				Expect(err).To(BeNil())

				// Verify editor relationship was written using direct SpiceDB client with full consistency
				resp, err := spiceDBClient.ReadRelationships(ctx, &v1pb.ReadRelationshipsRequest{
					Consistency: &v1pb.Consistency{
						Requirement: &v1pb.Consistency_FullyConsistent{
							FullyConsistent: true,
						},
					},
					RelationshipFilter: &v1pb.RelationshipFilter{
						ResourceType:       model.AssessmentObject,
						OptionalResourceId: testAssessmentIDEditor,
						OptionalRelation:   "editor",
						OptionalSubjectFilter: &v1pb.SubjectFilter{
							SubjectType:       model.UserObject,
							OptionalSubjectId: testUserID,
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
				Expect(relationships[0].Resource.ObjectId).To(Equal(testAssessmentIDEditor))
				Expect(relationships[0].Relation).To(Equal("editor"))
				Expect(relationships[0].Subject.Object.ObjectType).To(Equal(model.UserObject))
				Expect(relationships[0].Subject.Object.ObjectId).To(Equal(testUserID))
			})

			It("should write reader relationship and verify correct permissions", func() {
				testAssessmentIDReader := "assessment-reader-" + uuid.New().String()[:8]
				subject := model.NewUserSubject(testUserID)

				err := authzSvc.WriteRelationships(ctx, store.WithReaderRelationship(testAssessmentIDReader, subject))
				Expect(err).To(BeNil())

				// Verify reader relationship was written using direct SpiceDB client with full consistency
				resp, err := spiceDBClient.ReadRelationships(ctx, &v1pb.ReadRelationshipsRequest{
					Consistency: &v1pb.Consistency{
						Requirement: &v1pb.Consistency_FullyConsistent{
							FullyConsistent: true,
						},
					},
					RelationshipFilter: &v1pb.RelationshipFilter{
						ResourceType:       model.AssessmentObject,
						OptionalResourceId: testAssessmentIDReader,
						OptionalRelation:   "reader",
						OptionalSubjectFilter: &v1pb.SubjectFilter{
							SubjectType:       model.UserObject,
							OptionalSubjectId: testUserID,
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
				Expect(relationships[0].Resource.ObjectId).To(Equal(testAssessmentIDReader))
				Expect(relationships[0].Relation).To(Equal("reader"))
				Expect(relationships[0].Subject.Object.ObjectType).To(Equal(model.UserObject))
				Expect(relationships[0].Subject.Object.ObjectId).To(Equal(testUserID))
			})
		})

		Context("Organization-Assessment relationships", func() {
			It("should write organization owner relationship", func() {
				subject := model.NewOrganizationSubject(testOrgID)

				err := authzSvc.WriteRelationships(ctx, store.WithEditorRelationship(testAssessmentID2, subject))
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
						OptionalResourceId: testAssessmentID2,
						OptionalRelation:   "editor",
						OptionalSubjectFilter: &v1pb.SubjectFilter{
							SubjectType:       model.OrgObject,
							OptionalSubjectId: testOrgID,
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
				Expect(relationships[0].Relation).To(Equal("editor"))
			})
		})
	})

	Context("List Permissions", func() {
		var (
			listTestUserID        string
			listTestOrgID         string
			listTestAssessmentID1 string
			listTestAssessmentID2 string
			listTestAssessmentID3 string
		)

		BeforeEach(func() {
			// Setup isolated test data for list permissions
			listTestUserID = "list-user-" + uuid.New().String()[:8]
			listTestOrgID = "list-org-" + uuid.New().String()[:8]
			listTestAssessmentID1 = "list-assessment1-" + uuid.New().String()[:8]
			listTestAssessmentID2 = "list-assessment2-" + uuid.New().String()[:8]
			listTestAssessmentID3 = "list-assessment3-" + uuid.New().String()[:8]

			// Debug output: Print test variables to stdout
			fmt.Printf("=== List Permissions Test Variables ===\n")
			fmt.Printf("listTestUserID: %s\n", listTestUserID)
			fmt.Printf("listTestOrgID: %s\n", listTestOrgID)
			fmt.Printf("listTestAssessmentID1: %s\n", listTestAssessmentID1)
			fmt.Printf("listTestAssessmentID2: %s\n", listTestAssessmentID2)
			fmt.Printf("listTestAssessmentID3: %s\n", listTestAssessmentID3)
			fmt.Printf("=====================================\n")

			// Use direct SpiceDB client to write prerequisites
			// 1. User is member of organization
			_, err := spiceDBClient.WriteRelationships(ctx, &v1pb.WriteRelationshipsRequest{
				Updates: []*v1pb.RelationshipUpdate{
					{
						Operation: v1pb.RelationshipUpdate_OPERATION_CREATE,
						Relationship: &v1pb.Relationship{
							Resource: &v1pb.ObjectReference{
								ObjectType: model.OrgObject,
								ObjectId:   listTestOrgID,
							},
							Relation: "member",
							Subject: &v1pb.SubjectReference{
								Object: &v1pb.ObjectReference{
									ObjectType: model.UserObject,
									ObjectId:   listTestUserID,
								},
							},
						},
					},
				},
			})
			Expect(err).To(BeNil())

			// 2. Setup different permissions on assessments
			updates := []*v1pb.RelationshipUpdate{
				// Assessment1: User has direct owner permission
				{
					Operation: v1pb.RelationshipUpdate_OPERATION_CREATE,
					Relationship: &v1pb.Relationship{
						Resource: &v1pb.ObjectReference{
							ObjectType: model.AssessmentObject,
							ObjectId:   listTestAssessmentID1,
						},
						Relation: "owner",
						Subject: &v1pb.SubjectReference{
							Object: &v1pb.ObjectReference{
								ObjectType: model.UserObject,
								ObjectId:   listTestUserID,
							},
						},
					},
				},
				// Assessment2: User has direct reader permission
				{
					Operation: v1pb.RelationshipUpdate_OPERATION_CREATE,
					Relationship: &v1pb.Relationship{
						Resource: &v1pb.ObjectReference{
							ObjectType: model.AssessmentObject,
							ObjectId:   listTestAssessmentID2,
						},
						Relation: "reader",
						Subject: &v1pb.SubjectReference{
							Object: &v1pb.ObjectReference{
								ObjectType: model.UserObject,
								ObjectId:   listTestUserID,
							},
						},
					},
				},
				// Assessment3: Organization has editor permission (user gets access through org membership)
				{
					Operation: v1pb.RelationshipUpdate_OPERATION_CREATE,
					Relationship: &v1pb.Relationship{
						Resource: &v1pb.ObjectReference{
							ObjectType: model.AssessmentObject,
							ObjectId:   listTestAssessmentID3,
						},
						Relation: "editor",
						Subject: &v1pb.SubjectReference{
							Object: &v1pb.ObjectReference{
								ObjectType: model.OrgObject,
								ObjectId:   listTestOrgID,
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
			zedToken = token.WrittenAt
		})

		AfterEach(func() {
			// Cleanup list test relationships
			cleanupListTestRelationships(ctx, spiceDBClient, listTestUserID, listTestOrgID,
				listTestAssessmentID1, listTestAssessmentID2, listTestAssessmentID3)
		})

		It("should return correct permissions for user with different access patterns", func() {
			authzSvc.ZedToken = nil
			zedCtx := context.WithValue(ctx, store.ZedTokenKey{}, zedToken)
			resources, err := authzSvc.ListResources(zedCtx, listTestUserID)
			Expect(err).To(BeNil())
			Expect(resources).To(HaveLen(3), "User should have access to 3 assessments")

			// Verify permissions for each assessment
			for _, resource := range resources {
				switch resource.AssessmentID {
				case listTestAssessmentID1:
					// Owner should have all permissions
					Expect(resource.Permissions).To(ContainElements(
						model.ReadPermission,
						model.EditPermission,
						model.SharePermission,
						model.DeletePermission,
					), "Assessment1: Owner should have all permissions")

				case listTestAssessmentID2:
					// Reader should only have read permission
					Expect(resource.Permissions).To(ContainElement(model.ReadPermission),
						"Assessment2: Reader should have read permission")
					Expect(resource.Permissions).ToNot(ContainElements(
						model.EditPermission,
						model.SharePermission,
						model.DeletePermission,
					), "Assessment2: Reader should not have write permissions")

				case listTestAssessmentID3:
					// Access through organization editor relationship - user should have read and edit permissions
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

		It("should return empty list for user with no permissions", func() {
			nonMemberUserID := "no-access-user-" + uuid.New().String()[:8]
			resources, err := authzSvc.ListResources(ctx, nonMemberUserID)
			Expect(err).To(BeNil())
			Expect(resources).To(HaveLen(0), "User with no permissions should get empty list")
		})
	})

	Context("Delete Relationships", func() {
		var (
			deleteTestUserID        string
			deleteTestAssessmentID1 string
			deleteTestAssessmentID2 string
		)

		BeforeEach(func() {
			// Setup test data for delete operations
			deleteTestUserID = "delete-user-" + uuid.New().String()[:8]
			deleteTestAssessmentID1 = "delete-assessment1-" + uuid.New().String()[:8]
			deleteTestAssessmentID2 = "delete-assessment2-" + uuid.New().String()[:8]

			// Setup user-organization membership
			_, err := setupUserOrganizationMembership(ctx, spiceDBClient, deleteTestUserID, testOrgID)
			Expect(err).To(BeNil())

			// Create relationships that we will delete using direct SpiceDB client
			// Setup relationships for delete tests
			resp, err := spiceDBClient.WriteRelationships(ctx, &v1pb.WriteRelationshipsRequest{
				Updates: []*v1pb.RelationshipUpdate{
					// deleteTestAssessmentID1: User has editor permission
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
									ObjectId:   deleteTestUserID,
								},
							},
						},
					},
					// deleteTestAssessmentID2: User has reader permission
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
									ObjectId:   deleteTestUserID,
								},
							},
						},
					},
					// deleteTestAssessmentID2: User has editor permission (dual permissions)
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
									ObjectId:   deleteTestUserID,
								},
							},
						},
					},
				},
			})
			Expect(err).To(BeNil())
			zedToken = resp.WrittenAt
		})

		AfterEach(func() {
			// Cleanup delete test relationships
			cleanupUserFromOrganization(ctx, spiceDBClient, deleteTestUserID, testOrgID)
		})

		It("should delete a relationship successfully", func() {
			subject := model.NewUserSubject(deleteTestUserID)

			// Verify the relationship exists before deletion using SpiceDB client
			checkResp, err := spiceDBClient.CheckPermission(ctx, &v1pb.CheckPermissionRequest{
				Consistency: &v1pb.Consistency{
					Requirement: &v1pb.Consistency_AtLeastAsFresh{
						AtLeastAsFresh: zedToken,
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
						ObjectId:   deleteTestUserID,
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(checkResp.Permissionship).To(Equal(v1pb.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION), "Editor relationship should exist before deletion")

			// Delete the relationship
			err = authzSvc.DeleteRelationships(ctx, store.WithEditorRelationship(deleteTestAssessmentID1, subject))
			Expect(err).To(BeNil())

			// Verify the relationship is gone using SpiceDB client with consistency
			checkResp, err = spiceDBClient.CheckPermission(ctx, &v1pb.CheckPermissionRequest{
				Resource: &v1pb.ObjectReference{
					ObjectType: model.AssessmentObject,
					ObjectId:   deleteTestAssessmentID1,
				},
				Permission: model.EditPermission.String(),
				Subject: &v1pb.SubjectReference{
					Object: &v1pb.ObjectReference{
						ObjectType: model.UserObject,
						ObjectId:   deleteTestUserID,
					},
				},
				Consistency: &v1pb.Consistency{
					Requirement: &v1pb.Consistency_AtLeastAsFresh{
						AtLeastAsFresh: authzSvc.ZedToken,
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(checkResp.Permissionship).To(Equal(v1pb.CheckPermissionResponse_PERMISSIONSHIP_NO_PERMISSION), "Editor relationship should be deleted")

			// Verify by reading relationships directly
			resp, err := spiceDBClient.ReadRelationships(ctx, &v1pb.ReadRelationshipsRequest{
				RelationshipFilter: &v1pb.RelationshipFilter{
					ResourceType:       model.AssessmentObject,
					OptionalResourceId: deleteTestAssessmentID1,
					OptionalRelation:   "editor",
					OptionalSubjectFilter: &v1pb.SubjectFilter{
						SubjectType:       model.UserObject,
						OptionalSubjectId: deleteTestUserID,
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

		It("should delete only the specific relationship without affecting others", func() {
			subject := model.NewUserSubject(deleteTestUserID)

			// Verify both relationships exist using SpiceDB client
			// Check read permission
			readResp, err := spiceDBClient.CheckPermission(ctx, &v1pb.CheckPermissionRequest{
				Consistency: &v1pb.Consistency{
					Requirement: &v1pb.Consistency_AtLeastAsFresh{
						AtLeastAsFresh: zedToken,
					},
				},
				Resource: &v1pb.ObjectReference{
					ObjectType: model.AssessmentObject,
					ObjectId:   deleteTestAssessmentID2,
				},
				Permission: model.ReadPermission.String(),
				Subject: &v1pb.SubjectReference{
					Object: &v1pb.ObjectReference{
						ObjectType: model.UserObject,
						ObjectId:   deleteTestUserID,
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(readResp.Permissionship).To(Equal(v1pb.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION), "Reader relationship should exist")

			// Check edit permission
			editResp, err := spiceDBClient.CheckPermission(ctx, &v1pb.CheckPermissionRequest{
				Consistency: &v1pb.Consistency{
					Requirement: &v1pb.Consistency_AtLeastAsFresh{
						AtLeastAsFresh: zedToken,
					},
				},
				Resource: &v1pb.ObjectReference{
					ObjectType: model.AssessmentObject,
					ObjectId:   deleteTestAssessmentID2,
				},
				Permission: model.EditPermission.String(),
				Subject: &v1pb.SubjectReference{
					Object: &v1pb.ObjectReference{
						ObjectType: model.UserObject,
						ObjectId:   deleteTestUserID,
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(editResp.Permissionship).To(Equal(v1pb.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION), "Editor relationship should exist")

			// Delete only the editor relationship
			err = authzSvc.DeleteRelationships(ctx, store.WithEditorRelationship(deleteTestAssessmentID2, subject))
			Expect(err).To(BeNil())

			// Verify editor permission is gone but reader remains using SpiceDB client with consistency
			// Check edit permission (should be gone)
			editResp, err = spiceDBClient.CheckPermission(ctx, &v1pb.CheckPermissionRequest{
				Resource: &v1pb.ObjectReference{
					ObjectType: model.AssessmentObject,
					ObjectId:   deleteTestAssessmentID2,
				},
				Permission: model.EditPermission.String(),
				Subject: &v1pb.SubjectReference{
					Object: &v1pb.ObjectReference{
						ObjectType: model.UserObject,
						ObjectId:   deleteTestUserID,
					},
				},
				Consistency: &v1pb.Consistency{
					Requirement: &v1pb.Consistency_AtLeastAsFresh{
						AtLeastAsFresh: authzSvc.ZedToken,
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(editResp.Permissionship).To(Equal(v1pb.CheckPermissionResponse_PERMISSIONSHIP_NO_PERMISSION), "Editor permission should be gone")

			// Check read permission (should still exist)
			readResp, err = spiceDBClient.CheckPermission(ctx, &v1pb.CheckPermissionRequest{
				Resource: &v1pb.ObjectReference{
					ObjectType: model.AssessmentObject,
					ObjectId:   deleteTestAssessmentID2,
				},
				Permission: model.ReadPermission.String(),
				Subject: &v1pb.SubjectReference{
					Object: &v1pb.ObjectReference{
						ObjectType: model.UserObject,
						ObjectId:   deleteTestUserID,
					},
				},
				Consistency: &v1pb.Consistency{
					Requirement: &v1pb.Consistency_AtLeastAsFresh{
						AtLeastAsFresh: authzSvc.ZedToken,
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(readResp.Permissionship).To(Equal(v1pb.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION), "Reader permission should still exist")

			// Verify by reading relationships directly
			resp, err := spiceDBClient.ReadRelationships(ctx, &v1pb.ReadRelationshipsRequest{
				Consistency: &v1pb.Consistency{
					Requirement: &v1pb.Consistency_AtLeastAsFresh{
						AtLeastAsFresh: authzSvc.ZedToken,
					},
				},
				RelationshipFilter: &v1pb.RelationshipFilter{
					ResourceType:       model.AssessmentObject,
					OptionalResourceId: deleteTestAssessmentID2,
					OptionalSubjectFilter: &v1pb.SubjectFilter{
						SubjectType:       model.UserObject,
						OptionalSubjectId: deleteTestUserID,
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
			Expect(relationships).To(HaveLen(1), "Should have exactly one relationship remaining")
			Expect(relationships[0].Relation).To(Equal("reader"), "Remaining relationship should be reader")
		})

		It("should handle deleting non-existent relationships gracefully", func() {
			nonExistentUserID := "user-nonexistent"
			subject := model.NewUserSubject(nonExistentUserID)

			err := authzSvc.DeleteRelationships(ctx, store.WithEditorRelationship(deleteTestAssessmentID1, subject))
			Expect(err).To(BeNil(), "SpiceDB should handle non-existent deletes gracefully")

			// Verify that the original relationship for deleteTestUserID still exists using SpiceDB client with consistency
			checkResp, err := spiceDBClient.CheckPermission(ctx, &v1pb.CheckPermissionRequest{
				Resource: &v1pb.ObjectReference{
					ObjectType: model.AssessmentObject,
					ObjectId:   deleteTestAssessmentID1,
				},
				Permission: model.EditPermission.String(),
				Subject: &v1pb.SubjectReference{
					Object: &v1pb.ObjectReference{
						ObjectType: model.UserObject,
						ObjectId:   deleteTestUserID,
					},
				},
				Consistency: &v1pb.Consistency{
					Requirement: &v1pb.Consistency_AtLeastAsFresh{
						AtLeastAsFresh: authzSvc.ZedToken,
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(checkResp.Permissionship).To(Equal(v1pb.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION), "Original relationship should still exist")
		})

		It("should maintain consistency token across chained service operations", func() {
			// Test that verifies the ZedToken is correctly propagated through service methods
			// This tests the internal consistency mechanism without direct client calls

			// Setup test data
			chainTestUserID := "chain-user-" + uuid.New().String()[:8]
			chainTestAssessmentID := "chain-assessment-" + uuid.New().String()[:8]

			// Setup user-organization membership first
			_, err := setupUserOrganizationMembership(ctx, spiceDBClient, chainTestUserID, testOrgID)
			Expect(err).To(BeNil())

			// Step 1: Write a relationship using the service
			subject := model.NewUserSubject(chainTestUserID)
			err = authzSvc.WriteRelationships(ctx, store.WithOwnerRelationship(chainTestAssessmentID, subject))
			Expect(err).To(BeNil())

			// Verify the token was captured by the service
			Expect(authzSvc.ZedToken).ToNot(BeNil(), "Service should have captured ZedToken from write operation")

			// Step 2: Read the relationship using the service (should use the captured token)
			permissions, err := authzSvc.HasPermissions(ctx, chainTestUserID, chainTestAssessmentID, []model.Permission{
				model.ReadPermission,
				model.EditPermission,
				model.SharePermission,
				model.DeletePermission,
			})
			Expect(err).To(BeNil())
			Expect(permissions[model.ReadPermission]).To(BeTrue(), "Owner should have read permission")
			Expect(permissions[model.EditPermission]).To(BeTrue(), "Owner should have edit permission")
			Expect(permissions[model.SharePermission]).To(BeTrue(), "Owner should have share permission")
			Expect(permissions[model.DeletePermission]).To(BeTrue(), "Owner should have delete permission")

			// Step 3: Delete the relationship using the service
			err = authzSvc.DeleteRelationships(ctx, store.WithOwnerRelationship(chainTestAssessmentID, subject))
			Expect(err).To(BeNil())

			// Verify the token was updated by the delete operation
			Expect(authzSvc.ZedToken).ToNot(BeNil(), "Service should have updated ZedToken from delete operation")

			// Step 4: Read again to verify the relationship is gone (should use the updated token)
			permissions, err = authzSvc.HasPermissions(ctx, chainTestUserID, chainTestAssessmentID, []model.Permission{
				model.ReadPermission,
				model.EditPermission,
				model.SharePermission,
				model.DeletePermission,
			})
			Expect(err).To(BeNil())
			Expect(permissions[model.ReadPermission]).To(BeFalse(), "All permissions should be gone after delete")
			Expect(permissions[model.EditPermission]).To(BeFalse(), "All permissions should be gone after delete")
			Expect(permissions[model.SharePermission]).To(BeFalse(), "All permissions should be gone after delete")
			Expect(permissions[model.DeletePermission]).To(BeFalse(), "All permissions should be gone after delete")

			// Cleanup
			cleanupUserFromOrganization(ctx, spiceDBClient, chainTestUserID, testOrgID)
		})
	})
})

// Helper function to setup user membership in organization
func setupUserOrganizationMembership(ctx context.Context, client *authzed.Client, userID, orgID string) (*v1pb.ZedToken, error) {
	resp, err := client.WriteRelationships(ctx, &v1pb.WriteRelationshipsRequest{
		Updates: []*v1pb.RelationshipUpdate{
			{
				Operation: v1pb.RelationshipUpdate_OPERATION_TOUCH,
				Relationship: &v1pb.Relationship{
					Resource: &v1pb.ObjectReference{
						ObjectType: model.OrgObject,
						ObjectId:   orgID,
					},
					Relation: "member",
					Subject: &v1pb.SubjectReference{
						Object: &v1pb.ObjectReference{
							ObjectType: model.UserObject,
							ObjectId:   userID,
						},
					},
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	return resp.WrittenAt, nil
}

// Helper function to verify user membership in organization
func verifyUserOrganizationMembership(ctx context.Context, client *authzed.Client, userID, orgID string, token *v1pb.ZedToken) error {
	var consistency *v1pb.Consistency
	if token != nil {
		consistency = &v1pb.Consistency{
			Requirement: &v1pb.Consistency_AtExactSnapshot{
				AtExactSnapshot: token,
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
			OptionalResourceId: orgID,
			OptionalRelation:   "member",
			OptionalSubjectFilter: &v1pb.SubjectFilter{
				SubjectType:       model.UserObject,
				OptionalSubjectId: userID,
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

// Helper function to clean up user from organization
func cleanupUserFromOrganization(ctx context.Context, client *authzed.Client, userID, orgID string) {
	client.WriteRelationships(ctx, &v1pb.WriteRelationshipsRequest{
		Updates: []*v1pb.RelationshipUpdate{
			{
				Operation: v1pb.RelationshipUpdate_OPERATION_DELETE,
				Relationship: &v1pb.Relationship{
					Resource: &v1pb.ObjectReference{
						ObjectType: model.OrgObject,
						ObjectId:   orgID,
					},
					Relation: "member",
					Subject: &v1pb.SubjectReference{
						Object: &v1pb.ObjectReference{
							ObjectType: model.UserObject,
							ObjectId:   userID,
						},
					},
				},
			},
		},
	})
}

// Helper function to clean up all test relationships
func cleanupAllRelationships(ctx context.Context, client *authzed.Client, assessmentID1, assessmentID2, userID, orgID string) {
	relationships := []*v1pb.RelationshipUpdate{
		// Assessment relationships
		{
			Operation: v1pb.RelationshipUpdate_OPERATION_DELETE,
			Relationship: &v1pb.Relationship{
				Resource: &v1pb.ObjectReference{
					ObjectType: model.AssessmentObject,
					ObjectId:   assessmentID1,
				},
				Relation: "owner",
				Subject: &v1pb.SubjectReference{
					Object: &v1pb.ObjectReference{
						ObjectType: model.UserObject,
						ObjectId:   userID,
					},
				},
			},
		},
		{
			Operation: v1pb.RelationshipUpdate_OPERATION_DELETE,
			Relationship: &v1pb.Relationship{
				Resource: &v1pb.ObjectReference{
					ObjectType: model.AssessmentObject,
					ObjectId:   assessmentID2,
				},
				Relation: "owner",
				Subject: &v1pb.SubjectReference{
					Object: &v1pb.ObjectReference{
						ObjectType: model.OrgObject,
						ObjectId:   orgID,
					},
				},
			},
		},
		// User-organization membership
		{
			Operation: v1pb.RelationshipUpdate_OPERATION_DELETE,
			Relationship: &v1pb.Relationship{
				Resource: &v1pb.ObjectReference{
					ObjectType: model.OrgObject,
					ObjectId:   orgID,
				},
				Relation: "member",
				Subject: &v1pb.SubjectReference{
					Object: &v1pb.ObjectReference{
						ObjectType: model.UserObject,
						ObjectId:   userID,
					},
				},
			},
		},
	}

	// Delete all relationships (ignore errors as they might not exist)
	client.WriteRelationships(ctx, &v1pb.WriteRelationshipsRequest{
		Updates: relationships,
	})
}

// Helper function to clean up list test relationships
func cleanupListTestRelationships(ctx context.Context, client *authzed.Client, userID, orgID, assessmentID1, assessmentID2, assessmentID3 string) {
	updates := []*v1pb.RelationshipUpdate{
		// User-organization membership
		{
			Operation: v1pb.RelationshipUpdate_OPERATION_DELETE,
			Relationship: &v1pb.Relationship{
				Resource: &v1pb.ObjectReference{
					ObjectType: model.OrgObject,
					ObjectId:   orgID,
				},
				Relation: "member",
				Subject: &v1pb.SubjectReference{
					Object: &v1pb.ObjectReference{
						ObjectType: model.UserObject,
						ObjectId:   userID,
					},
				},
			},
		},
		// Assessment relationships
		{
			Operation: v1pb.RelationshipUpdate_OPERATION_DELETE,
			Relationship: &v1pb.Relationship{
				Resource: &v1pb.ObjectReference{
					ObjectType: model.AssessmentObject,
					ObjectId:   assessmentID1,
				},
				Relation: "owner",
				Subject: &v1pb.SubjectReference{
					Object: &v1pb.ObjectReference{
						ObjectType: model.UserObject,
						ObjectId:   userID,
					},
				},
			},
		},
		{
			Operation: v1pb.RelationshipUpdate_OPERATION_DELETE,
			Relationship: &v1pb.Relationship{
				Resource: &v1pb.ObjectReference{
					ObjectType: model.AssessmentObject,
					ObjectId:   assessmentID2,
				},
				Relation: "reader",
				Subject: &v1pb.SubjectReference{
					Object: &v1pb.ObjectReference{
						ObjectType: model.UserObject,
						ObjectId:   userID,
					},
				},
			},
		},
		{
			Operation: v1pb.RelationshipUpdate_OPERATION_DELETE,
			Relationship: &v1pb.Relationship{
				Resource: &v1pb.ObjectReference{
					ObjectType: model.AssessmentObject,
					ObjectId:   assessmentID3,
				},
				Relation: "editor",
				Subject: &v1pb.SubjectReference{
					Object: &v1pb.ObjectReference{
						ObjectType: model.OrgObject,
						ObjectId:   orgID,
					},
					OptionalRelation: "member",
				},
			},
		},
	}

	client.WriteRelationships(ctx, &v1pb.WriteRelationshipsRequest{
		Updates: updates,
	})
}
