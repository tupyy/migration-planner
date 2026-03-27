package service_test

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
)

const (
	insertAccountsOrgStm  = "INSERT INTO organizations (id, name, description, kind, company, parent_id) VALUES ('%s', '%s', '%s', '%s', '%s', %s);"
	insertAccountsUserStm = "INSERT INTO users (id, username, email, first_name, last_name, phone, location, title, bio, organization_id) VALUES ('%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s');"
)

var _ = Describe("accounts service", Ordered, func() {
	var (
		s      store.Store
		gormdb *gorm.DB
		svc    *service.AccountsService
	)

	BeforeAll(func() {
		cfg, err := config.New()
		Expect(err).To(BeNil())
		db, err := store.InitDB(cfg)
		Expect(err).To(BeNil())

		s = store.NewStore(db)
		gormdb = db
		svc = service.NewAccountsService(s)
	})

	AfterAll(func() {
		_ = s.Close()
	})

	Context("GetIdentity", func() {
		It("returns regular identity from JWT when user not in DB", func() {
			authUser := auth.User{
				Username:     "jwtuser",
				Organization: "jwt-org-id",
			}

			identity, err := svc.GetIdentity(context.TODO(), authUser)
			Expect(err).To(BeNil())
			Expect(identity.Username).To(Equal("jwtuser"))
			Expect(identity.Kind).To(Equal("regular"))
			Expect(identity.OrganizationID).To(Equal("jwt-org-id"))
		})

		It("returns admin identity when user belongs to admin org", func() {
			orgID := uuid.New()
			userID := uuid.New()

			tx := gormdb.Exec(fmt.Sprintf(insertAccountsOrgStm, orgID, "Admin Org", "desc", "admin", "Red Hat", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAccountsUserStm, userID, "adminuser", "admin@rh.com", "Admin", "User", "+1-555-0001", "na", "", "", orgID))
			Expect(tx.Error).To(BeNil())

			authUser := auth.User{
				Username:     "adminuser",
				Organization: "jwt-org-id",
			}

			identity, err := svc.GetIdentity(context.TODO(), authUser)
			Expect(err).To(BeNil())
			Expect(identity.Username).To(Equal("adminuser"))
			Expect(identity.Kind).To(Equal("admin"))
			Expect(identity.OrganizationID).To(Equal(orgID.String()))
		})

		It("returns partner identity when user belongs to partner org", func() {
			orgID := uuid.New()
			userID := uuid.New()

			tx := gormdb.Exec(fmt.Sprintf(insertAccountsOrgStm, orgID, "Partner Org", "desc", "partner", "Acme", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAccountsUserStm, userID, "partneruser", "partner@acme.com", "Partner", "User", "+1-555-0001", "na", "", "", orgID))
			Expect(tx.Error).To(BeNil())

			authUser := auth.User{
				Username:     "partneruser",
				Organization: "jwt-org-id",
			}

			identity, err := svc.GetIdentity(context.TODO(), authUser)
			Expect(err).To(BeNil())
			Expect(identity.Username).To(Equal("partneruser"))
			Expect(identity.Kind).To(Equal("partner"))
			Expect(identity.OrganizationID).To(Equal(orgID.String()))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM users;")
			gormdb.Exec("DELETE FROM organizations;")
		})
	})

	Context("Organizations", func() {
		Context("GetOrganization", func() {
			It("returns the organization", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertAccountsOrgStm, orgID, "Test Org", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				org, err := svc.GetOrganization(context.TODO(), orgID)
				Expect(err).To(BeNil())
				Expect(org.ID).To(Equal(orgID))
				Expect(org.Name).To(Equal("Test Org"))
			})

			It("returns ErrResourceNotFound for missing org", func() {
				_, err := svc.GetOrganization(context.TODO(), uuid.New())
				Expect(err).ToNot(BeNil())
				var notFound *service.ErrResourceNotFound
				Expect(err).To(BeAssignableToTypeOf(notFound))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM organizations;")
			})
		})

		Context("CreateOrganization", func() {
			It("creates an organization", func() {
				orgID := uuid.New()
				org := model.Organization{
					ID:   orgID,
					Name: "New Org",
					Kind: "partner",
				}

				created, err := svc.CreateOrganization(context.TODO(), org)
				Expect(err).To(BeNil())
				Expect(created.ID).To(Equal(orgID))

				var count int
				tx := gormdb.Raw("SELECT COUNT(*) FROM organizations;").Scan(&count)
				Expect(tx.Error).To(BeNil())
				Expect(count).To(Equal(1))
			})

			It("returns ErrDuplicateKey for duplicate org name", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertAccountsOrgStm, orgID, "Existing Org", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				org := model.Organization{
					ID:   orgID,
					Name: "Existing Org",
					Kind: "partner",
				}
				_, err := svc.CreateOrganization(context.TODO(), org)
				Expect(err).ToNot(BeNil())
				var dupKey *service.ErrDuplicateKey
				Expect(err).To(BeAssignableToTypeOf(dupKey))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM organizations;")
			})
		})

		Context("UpdateOrganization", func() {
			It("updates an organization", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertAccountsOrgStm, orgID, "Old Name", "old desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				org := model.Organization{
					ID:          orgID,
					Name:        "New Name",
					Description: "new desc",
					Kind:        "partner",
					Company:     "Acme",
				}
				updated, err := svc.UpdateOrganization(context.TODO(), org)
				Expect(err).To(BeNil())
				Expect(updated.Name).To(Equal("New Name"))
			})

			It("returns ErrResourceNotFound for missing org", func() {
				org := model.Organization{
					ID:   uuid.New(),
					Name: "Does Not Exist",
					Kind: "partner",
				}
				_, err := svc.UpdateOrganization(context.TODO(), org)
				Expect(err).ToNot(BeNil())
				var notFound *service.ErrResourceNotFound
				Expect(err).To(BeAssignableToTypeOf(notFound))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM organizations;")
			})
		})

		Context("DeleteOrganization", func() {
			It("deletes an organization", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertAccountsOrgStm, orgID, "To Delete", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				err := svc.DeleteOrganization(context.TODO(), orgID)
				Expect(err).To(BeNil())

				var count int
				tx = gormdb.Raw("SELECT COUNT(*) FROM organizations;").Scan(&count)
				Expect(tx.Error).To(BeNil())
				Expect(count).To(Equal(0))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM organizations;")
			})
		})
	})

	Context("Users", func() {
		Context("GetUser", func() {
			It("returns the user", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertAccountsOrgStm, orgID, "Test Org", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsUserStm, uuid.New(), "testuser", "test@acme.com", "Test", "User", "+1-555-0001", "na", "", "", orgID))
				Expect(tx.Error).To(BeNil())

				user, err := svc.GetUser(context.TODO(), "testuser")
				Expect(err).To(BeNil())
				Expect(user.Username).To(Equal("testuser"))
			})

			It("returns ErrResourceNotFound for missing user", func() {
				_, err := svc.GetUser(context.TODO(), "nonexistent")
				Expect(err).ToNot(BeNil())
				var notFound *service.ErrResourceNotFound
				Expect(err).To(BeAssignableToTypeOf(notFound))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM users;")
				gormdb.Exec("DELETE FROM organizations;")
			})
		})

		Context("CreateUser", func() {
			It("creates a user", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertAccountsOrgStm, orgID, "Test Org", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				user := model.User{
					Username:       "newuser",
					Email:          "new@acme.com",
					FirstName:      "New",
					LastName:       "User",
					Phone:          "+1-555-0001",
					Location:       "na",
					OrganizationID: orgID,
				}

				created, err := svc.CreateUser(context.TODO(), user)
				Expect(err).To(BeNil())
				Expect(created.Username).To(Equal("newuser"))
				Expect(created.ID).ToNot(Equal(uuid.Nil))

				var count int
				tx = gormdb.Raw("SELECT COUNT(*) FROM users;").Scan(&count)
				Expect(tx.Error).To(BeNil())
				Expect(count).To(Equal(1))
			})

			It("returns ErrDuplicateKey for duplicate username", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertAccountsOrgStm, orgID, "Test Org", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsUserStm, uuid.New(), "dupuser", "dup@acme.com", "Dup", "User", "+1-555-0001", "na", "", "", orgID))
				Expect(tx.Error).To(BeNil())

				user := model.User{
					Username:       "dupuser",
					Email:          "dup2@acme.com",
					FirstName:      "Dup",
					LastName:       "Two",
					Phone:          "+1-555-0002",
					Location:       "na",
					OrganizationID: orgID,
				}
				_, err := svc.CreateUser(context.TODO(), user)
				Expect(err).ToNot(BeNil())
				var dupKey *service.ErrDuplicateKey
				Expect(err).To(BeAssignableToTypeOf(dupKey))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM users;")
				gormdb.Exec("DELETE FROM organizations;")
			})
		})

		Context("UpdateUser", func() {
			It("updates a user", func() {
				orgID := uuid.New()
				userID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertAccountsOrgStm, orgID, "Test Org", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsUserStm, userID, "updateme", "old@acme.com", "Old", "Name", "+1-555-0001", "na", "", "", orgID))
				Expect(tx.Error).To(BeNil())

				user := model.User{
					ID:             userID,
					Username:       "updateme",
					Email:          "new@acme.com",
					FirstName:      "New",
					LastName:       "Name",
					Phone:          "+1-555-0001",
					Location:       "emea",
					OrganizationID: orgID,
				}
				updated, err := svc.UpdateUser(context.TODO(), user)
				Expect(err).To(BeNil())
				Expect(updated.Email).To(Equal("new@acme.com"))
				Expect(updated.Location).To(Equal("emea"))
			})

			It("returns ErrResourceNotFound for missing user", func() {
				user := model.User{
					ID:        uuid.New(),
					Username:  "ghost",
					Email:     "ghost@acme.com",
					FirstName: "Ghost",
					LastName:  "User",
					Phone:     "+1-555-0001",
					Location:  "na",
				}
				_, err := svc.UpdateUser(context.TODO(), user)
				Expect(err).ToNot(BeNil())
				var notFound *service.ErrResourceNotFound
				Expect(err).To(BeAssignableToTypeOf(notFound))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM users;")
				gormdb.Exec("DELETE FROM organizations;")
			})
		})

		Context("DeleteUser", func() {
			It("deletes a user by username", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertAccountsOrgStm, orgID, "Test Org", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsUserStm, uuid.New(), "todelete", "del@acme.com", "Del", "User", "+1-555-0001", "na", "", "", orgID))
				Expect(tx.Error).To(BeNil())

				err := svc.DeleteUser(context.TODO(), "todelete")
				Expect(err).To(BeNil())

				var count int
				tx = gormdb.Raw("SELECT COUNT(*) FROM users;").Scan(&count)
				Expect(tx.Error).To(BeNil())
				Expect(count).To(Equal(0))
			})

			It("returns ErrResourceNotFound for missing user", func() {
				err := svc.DeleteUser(context.TODO(), "nonexistent")
				Expect(err).ToNot(BeNil())
				var notFound *service.ErrResourceNotFound
				Expect(err).To(BeAssignableToTypeOf(notFound))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM users;")
				gormdb.Exec("DELETE FROM organizations;")
			})
		})
	})

	Context("Membership", func() {
		Context("AddUserToOrganization", func() {
			It("adds a user to an organization", func() {
				orgID := uuid.New()
				newOrgID := uuid.New()
				userID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertAccountsOrgStm, orgID, "Old Org", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsOrgStm, newOrgID, "New Org", "desc", "partner", "Globex", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsUserStm, userID, "joinuser", "join@acme.com", "Join", "User", "+1-555-0001", "na", "", "", orgID))
				Expect(tx.Error).To(BeNil())

				err := svc.AddUserToOrganization(context.TODO(), newOrgID, "joinuser")
				Expect(err).To(BeNil())

				var orgIDResult string
				tx = gormdb.Raw(fmt.Sprintf("SELECT organization_id FROM users WHERE id = '%s';", userID)).Scan(&orgIDResult)
				Expect(tx.Error).To(BeNil())
				Expect(orgIDResult).To(Equal(newOrgID.String()))
			})

			It("returns error when org does not exist", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertAccountsOrgStm, orgID, "Some Org", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsUserStm, uuid.New(), "orphanuser", "orphan@acme.com", "Orphan", "User", "+1-555-0001", "na", "", "", orgID))
				Expect(tx.Error).To(BeNil())

				err := svc.AddUserToOrganization(context.TODO(), uuid.New(), "orphanuser")
				Expect(err).ToNot(BeNil())
				var notFound *service.ErrResourceNotFound
				Expect(err).To(BeAssignableToTypeOf(notFound))
			})

			It("returns error when user does not exist", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertAccountsOrgStm, orgID, "Test Org", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				err := svc.AddUserToOrganization(context.TODO(), orgID, "ghostuser")
				Expect(err).ToNot(BeNil())
				var notFound *service.ErrResourceNotFound
				Expect(err).To(BeAssignableToTypeOf(notFound))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM users;")
				gormdb.Exec("DELETE FROM organizations;")
			})
		})

		Context("RemoveUserFromOrganization", func() {
			It("returns error when org does not exist", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertAccountsOrgStm, orgID, "Some Org", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsUserStm, uuid.New(), "someuser", "some@acme.com", "Some", "User", "+1-555-0001", "na", "", "", orgID))
				Expect(tx.Error).To(BeNil())

				err := svc.RemoveUserFromOrganization(context.TODO(), uuid.New(), "someuser")
				Expect(err).ToNot(BeNil())
				var notFound *service.ErrResourceNotFound
				Expect(err).To(BeAssignableToTypeOf(notFound))
			})

			It("returns error when user does not exist", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertAccountsOrgStm, orgID, "Test Org", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				err := svc.RemoveUserFromOrganization(context.TODO(), orgID, "ghostuser")
				Expect(err).ToNot(BeNil())
				var notFound *service.ErrResourceNotFound
				Expect(err).To(BeAssignableToTypeOf(notFound))
			})

			It("returns ErrMembershipMismatch when user belongs to different org", func() {
				orgID := uuid.New()
				otherOrgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertAccountsOrgStm, orgID, "Org A", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsOrgStm, otherOrgID, "Org B", "desc", "partner", "Globex", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsUserStm, uuid.New(), "wrongorguser", "wrong@acme.com", "Wrong", "Org", "+1-555-0001", "na", "", "", otherOrgID))
				Expect(tx.Error).To(BeNil())

				err := svc.RemoveUserFromOrganization(context.TODO(), orgID, "wrongorguser")
				Expect(err).ToNot(BeNil())
				var mismatch *service.ErrMembershipMismatch
				Expect(err).To(BeAssignableToTypeOf(mismatch))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM users;")
				gormdb.Exec("DELETE FROM organizations;")
			})
		})
	})
})
