package store_test

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
)

const (
	insertOrgStm  = "INSERT INTO organizations (id, name, description, kind, company, parent_id) VALUES ('%s', '%s', '%s', '%s', '%s', %s);"
	insertUserStm = "INSERT INTO users (id, username, email, first_name, last_name, phone, location, title, bio, organization_id) VALUES ('%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s');"
)

var _ = Describe("accounts store", Ordered, func() {
	var (
		s      store.Store
		gormdb *gorm.DB
	)

	BeforeAll(func() {
		cfg, err := config.New()
		Expect(err).To(BeNil())
		db, err := store.InitDB(cfg)
		Expect(err).To(BeNil())

		s = store.NewStore(db)
		gormdb = db
	})

	AfterAll(func() {
		_ = s.Close()
	})

	Context("organizations", func() {
		Context("list", func() {
			It("successfully lists all organizations", func() {
				orgID1 := uuid.New()
				orgID2 := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertOrgStm, orgID1, "Org One", "First org", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertOrgStm, orgID2, "Org Two", "Second org", "admin", "Red Hat", "NULL"))
				Expect(tx.Error).To(BeNil())

				orgs, err := s.Accounts().ListOrganizations(context.TODO(), store.NewOrganizationQueryFilter())
				Expect(err).To(BeNil())
				Expect(orgs).To(HaveLen(2))
			})

			It("lists organizations filtered by kind", func() {
				orgID1 := uuid.New()
				orgID2 := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertOrgStm, orgID1, "Partner Org", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertOrgStm, orgID2, "Admin Org", "desc", "admin", "Red Hat", "NULL"))
				Expect(tx.Error).To(BeNil())

				orgs, err := s.Accounts().ListOrganizations(context.TODO(), store.NewOrganizationQueryFilter().ByKind("partner"))
				Expect(err).To(BeNil())
				Expect(orgs).To(HaveLen(1))
				Expect(orgs[0].Kind).To(Equal("partner"))
			})

			It("lists organizations filtered by name", func() {
				orgID1 := uuid.New()
				orgID2 := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertOrgStm, orgID1, "Acme Consulting", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertOrgStm, orgID2, "Platform Admin", "desc", "admin", "Red Hat", "NULL"))
				Expect(tx.Error).To(BeNil())

				orgs, err := s.Accounts().ListOrganizations(context.TODO(), store.NewOrganizationQueryFilter().ByName("acme"))
				Expect(err).To(BeNil())
				Expect(orgs).To(HaveLen(1))
				Expect(orgs[0].Name).To(Equal("Acme Consulting"))
			})

			It("lists organizations filtered by company", func() {
				orgID1 := uuid.New()
				orgID2 := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertOrgStm, orgID1, "Org One", "desc", "partner", "Acme Corp", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertOrgStm, orgID2, "Org Two", "desc", "partner", "Globex Inc", "NULL"))
				Expect(tx.Error).To(BeNil())

				orgs, err := s.Accounts().ListOrganizations(context.TODO(), store.NewOrganizationQueryFilter().ByCompany("acme"))
				Expect(err).To(BeNil())
				Expect(orgs).To(HaveLen(1))
				Expect(orgs[0].Company).To(Equal("Acme Corp"))
			})

			It("lists organizations filtered by parent ID", func() {
				parentID := uuid.New()
				childID := uuid.New()
				otherID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertOrgStm, parentID, "Parent Org", "desc", "admin", "Red Hat", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertOrgStm, childID, "Child Org", "desc", "partner", "Acme", fmt.Sprintf("'%s'", parentID)))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertOrgStm, otherID, "Other Org", "desc", "partner", "Globex", "NULL"))
				Expect(tx.Error).To(BeNil())

				orgs, err := s.Accounts().ListOrganizations(context.TODO(), store.NewOrganizationQueryFilter().ByParentID(parentID))
				Expect(err).To(BeNil())
				Expect(orgs).To(HaveLen(1))
				Expect(orgs[0].Name).To(Equal("Child Org"))
			})

			It("lists organizations with users preloaded", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertOrgStm, orgID, "Org With Users", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertUserStm, uuid.New(), "user1", "user1@acme.com", "John", "Doe", "+1-555-0001", "na", "Engineer", "Some bio", orgID))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertUserStm, uuid.New(), "user2", "user2@acme.com", "Jane", "Smith", "+1-555-0002", "emea", "Manager", "", orgID))
				Expect(tx.Error).To(BeNil())

				orgs, err := s.Accounts().ListOrganizations(context.TODO(), store.NewOrganizationQueryFilter())
				Expect(err).To(BeNil())
				Expect(orgs).To(HaveLen(1))
				Expect(orgs[0].Users).To(HaveLen(2))
			})

			It("lists no organizations when none exist", func() {
				orgs, err := s.Accounts().ListOrganizations(context.TODO(), store.NewOrganizationQueryFilter())
				Expect(err).To(BeNil())
				Expect(orgs).To(HaveLen(0))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM users;")
				gormdb.Exec("DELETE FROM organizations;")
			})
		})

		Context("get", func() {
			It("successfully gets an organization", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertOrgStm, orgID, "Test Org", "A test org", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				org, err := s.Accounts().GetOrganization(context.TODO(), orgID)
				Expect(err).To(BeNil())
				Expect(org.ID).To(Equal(orgID))
				Expect(org.Name).To(Equal("Test Org"))
				Expect(org.Description).To(Equal("A test org"))
				Expect(org.Kind).To(Equal("partner"))
				Expect(org.Company).To(Equal("Acme"))
			})

			It("gets an organization with users preloaded", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertOrgStm, orgID, "Test Org", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertUserStm, uuid.New(), "testuser", "test@acme.com", "Test", "User", "+1-555-0001", "na", "", "", orgID))
				Expect(tx.Error).To(BeNil())

				org, err := s.Accounts().GetOrganization(context.TODO(), orgID)
				Expect(err).To(BeNil())
				Expect(org.Users).To(HaveLen(1))
				Expect(org.Users[0].Username).To(Equal("testuser"))
			})

			It("fails to get non-existent organization", func() {
				_, err := s.Accounts().GetOrganization(context.TODO(), uuid.New())
				Expect(err).To(Equal(store.ErrRecordNotFound))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM users;")
				gormdb.Exec("DELETE FROM organizations;")
			})
		})

		Context("create", func() {
			It("successfully creates an organization", func() {
				orgID := uuid.New()
				org := model.Organization{
					ID:          orgID,
					Name:        "New Org",
					Description: "A new org",
					Kind:        "partner",
					Company:     "Acme",
				}

				created, err := s.Accounts().CreateOrganization(context.TODO(), org)
				Expect(err).To(BeNil())
				Expect(created.ID).To(Equal(orgID))
				Expect(created.Name).To(Equal("New Org"))

				var count int
				tx := gormdb.Raw("SELECT COUNT(*) FROM organizations;").Scan(&count)
				Expect(tx.Error).To(BeNil())
				Expect(count).To(Equal(1))
			})

			It("successfully creates an organization with parent", func() {
				parentID := uuid.New()
				childID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertOrgStm, parentID, "Parent", "desc", "admin", "Red Hat", "NULL"))
				Expect(tx.Error).To(BeNil())

				child := model.Organization{
					ID:       childID,
					Name:     "Child",
					Kind:     "partner",
					ParentID: &parentID,
				}
				created, err := s.Accounts().CreateOrganization(context.TODO(), child)
				Expect(err).To(BeNil())
				Expect(created.ParentID).ToNot(BeNil())
				Expect(*created.ParentID).To(Equal(parentID))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM users;")
				gormdb.Exec("DELETE FROM organizations;")
			})
		})

		Context("update", func() {
			It("successfully updates an organization", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertOrgStm, orgID, "Original Name", "Original desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				org := model.Organization{
					ID:          orgID,
					Name:        "Updated Name",
					Description: "Updated desc",
					Kind:        "partner",
					Company:     "Acme",
				}
				updated, err := s.Accounts().UpdateOrganization(context.TODO(), org)
				Expect(err).To(BeNil())
				Expect(updated.Name).To(Equal("Updated Name"))
				Expect(updated.Description).To(Equal("Updated desc"))
				Expect(updated.UpdatedAt).ToNot(BeNil())
			})

			It("fails to update non-existent organization", func() {
				org := model.Organization{
					ID:   uuid.New(),
					Name: "Does Not Exist",
					Kind: "partner",
				}
				_, err := s.Accounts().UpdateOrganization(context.TODO(), org)
				Expect(err).To(Equal(store.ErrRecordNotFound))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM users;")
				gormdb.Exec("DELETE FROM organizations;")
			})
		})

		Context("delete", func() {
			It("successfully deletes an organization", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertOrgStm, orgID, "To Delete", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				err := s.Accounts().DeleteOrganization(context.TODO(), orgID)
				Expect(err).To(BeNil())

				var count int
				tx = gormdb.Raw("SELECT COUNT(*) FROM organizations;").Scan(&count)
				Expect(tx.Error).To(BeNil())
				Expect(count).To(Equal(0))
			})

			It("does not fail when deleting non-existent organization", func() {
				err := s.Accounts().DeleteOrganization(context.TODO(), uuid.New())
				Expect(err).To(BeNil())
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM users;")
				gormdb.Exec("DELETE FROM organizations;")
			})
		})
	})

	Context("users", func() {
		Context("list", func() {
			It("successfully lists all users", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertOrgStm, orgID, "Test Org", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertUserStm, uuid.New(), "user1", "user1@test.com", "John", "Doe", "+1-555-0001", "na", "Engineer", "", orgID))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertUserStm, uuid.New(), "user2", "user2@test.com", "Jane", "Smith", "+1-555-0002", "emea", "Manager", "", orgID))
				Expect(tx.Error).To(BeNil())

				users, err := s.Accounts().ListUsers(context.TODO(), store.NewUserQueryFilter())
				Expect(err).To(BeNil())
				Expect(users).To(HaveLen(2))
			})

			It("lists users filtered by organization ID", func() {
				orgID1 := uuid.New()
				orgID2 := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertOrgStm, orgID1, "Org One", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertOrgStm, orgID2, "Org Two", "desc", "partner", "Globex", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertUserStm, uuid.New(), "user1", "user1@test.com", "John", "Doe", "+1-555-0001", "na", "", "", orgID1))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertUserStm, uuid.New(), "user2", "user2@test.com", "Jane", "Smith", "+1-555-0002", "emea", "", "", orgID2))
				Expect(tx.Error).To(BeNil())

				users, err := s.Accounts().ListUsers(context.TODO(), store.NewUserQueryFilter().ByOrganizationID(orgID1))
				Expect(err).To(BeNil())
				Expect(users).To(HaveLen(1))
				Expect(users[0].Username).To(Equal("user1"))
			})

			It("lists users filtered by location", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertOrgStm, orgID, "Test Org", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertUserStm, uuid.New(), "user1", "user1@test.com", "John", "Doe", "+1-555-0001", "na", "", "", orgID))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertUserStm, uuid.New(), "user2", "user2@test.com", "Jane", "Smith", "+1-555-0002", "emea", "", "", orgID))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertUserStm, uuid.New(), "user3", "user3@test.com", "Carlos", "Silva", "+55-11-9999", "latam", "", "", orgID))
				Expect(tx.Error).To(BeNil())

				users, err := s.Accounts().ListUsers(context.TODO(), store.NewUserQueryFilter().ByLocation("emea"))
				Expect(err).To(BeNil())
				Expect(users).To(HaveLen(1))
				Expect(users[0].Username).To(Equal("user2"))
			})

			It("lists no users when none exist", func() {
				users, err := s.Accounts().ListUsers(context.TODO(), store.NewUserQueryFilter())
				Expect(err).To(BeNil())
				Expect(users).To(HaveLen(0))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM users;")
				gormdb.Exec("DELETE FROM organizations;")
			})
		})

		Context("get", func() {
			It("successfully gets a user", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertOrgStm, orgID, "Test Org", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertUserStm, uuid.New(), "testuser", "test@acme.com", "Test", "User", "+1-555-0001", "na", "Engineer", "A bio", orgID))
				Expect(tx.Error).To(BeNil())

				user, err := s.Accounts().GetUser(context.TODO(), "testuser")
				Expect(err).To(BeNil())
				Expect(user.ID).ToNot(Equal(uuid.Nil))
				Expect(user.Username).To(Equal("testuser"))
				Expect(user.Email).To(Equal("test@acme.com"))
				Expect(user.FirstName).To(Equal("Test"))
				Expect(user.LastName).To(Equal("User"))
				Expect(user.Phone).To(Equal("+1-555-0001"))
				Expect(user.Location).To(Equal("na"))
				Expect(user.Title).To(Equal("Engineer"))
				Expect(user.Bio).To(Equal("A bio"))
				Expect(user.OrganizationID).To(Equal(orgID))
			})

			It("fails to get non-existent user", func() {
				_, err := s.Accounts().GetUser(context.TODO(), "nonexistent")
				Expect(err).To(Equal(store.ErrRecordNotFound))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM users;")
				gormdb.Exec("DELETE FROM organizations;")
			})
		})

		Context("create", func() {
			It("successfully creates a user", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertOrgStm, orgID, "Test Org", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				user := model.User{
					Username:       "newuser",
					Email:          "new@acme.com",
					FirstName:      "New",
					LastName:       "User",
					Phone:          "+1-555-0001",
					Location:       "na",
					Title:          "Developer",
					Bio:            "Hello",
					OrganizationID: orgID,
				}

				created, err := s.Accounts().CreateUser(context.TODO(), user)
				Expect(err).To(BeNil())
				Expect(created.ID).ToNot(Equal(uuid.Nil))
				Expect(created.Username).To(Equal("newuser"))
				Expect(created.OrganizationID).To(Equal(orgID))

				var count int
				tx = gormdb.Raw("SELECT COUNT(*) FROM users;").Scan(&count)
				Expect(tx.Error).To(BeNil())
				Expect(count).To(Equal(1))
			})

			It("fails to create user with duplicate username", func() {
				orgID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertOrgStm, orgID, "Test Org", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertUserStm, uuid.New(), "dupuser", "dup1@acme.com", "First", "User", "+1-555-0001", "na", "", "", orgID))
				Expect(tx.Error).To(BeNil())

				user2 := model.User{
					Username:       "dupuser",
					Email:          "dup2@acme.com",
					FirstName:      "Second",
					LastName:       "User",
					Phone:          "+1-555-0002",
					Location:       "emea",
					OrganizationID: orgID,
				}
				_, err := s.Accounts().CreateUser(context.TODO(), user2)
				Expect(err).ToNot(BeNil())
				Expect(err).To(Equal(store.ErrDuplicateKey))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM users;")
				gormdb.Exec("DELETE FROM organizations;")
			})
		})

		Context("update", func() {
			It("successfully updates a user", func() {
				orgID := uuid.New()
				userID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertOrgStm, orgID, "Test Org", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertUserStm, userID, "updateuser", "old@acme.com", "Old", "Name", "+1-555-0001", "na", "", "", orgID))
				Expect(tx.Error).To(BeNil())

				user := model.User{
					ID:             userID,
					Username:       "updateuser",
					Email:          "new@acme.com",
					FirstName:      "New",
					LastName:       "Name",
					Phone:          "+1-555-0001",
					Location:       "emea",
					OrganizationID: orgID,
				}
				updated, err := s.Accounts().UpdateUser(context.TODO(), user)
				Expect(err).To(BeNil())
				Expect(updated.Email).To(Equal("new@acme.com"))
				Expect(updated.FirstName).To(Equal("New"))
				Expect(updated.Location).To(Equal("emea"))
				Expect(updated.UpdatedAt).ToNot(BeNil())
			})

			It("successfully updates username", func() {
				orgID := uuid.New()
				userID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertOrgStm, orgID, "Test Org", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertUserStm, userID, "oldname", "user@acme.com", "Test", "User", "+1-555-0001", "na", "", "", orgID))
				Expect(tx.Error).To(BeNil())

				user := model.User{
					ID:             userID,
					Username:       "newname",
					Email:          "user@acme.com",
					FirstName:      "Test",
					LastName:       "User",
					Phone:          "+1-555-0001",
					Location:       "na",
					OrganizationID: orgID,
				}
				updated, err := s.Accounts().UpdateUser(context.TODO(), user)
				Expect(err).To(BeNil())
				Expect(updated.Username).To(Equal("newname"))
				Expect(updated.ID).To(Equal(userID))

				// Verify old username no longer resolves
				var count int
				tx = gormdb.Raw("SELECT COUNT(*) FROM users WHERE username = 'oldname';").Scan(&count)
				Expect(tx.Error).To(BeNil())
				Expect(count).To(Equal(0))

				// Verify new username exists
				tx = gormdb.Raw("SELECT COUNT(*) FROM users WHERE username = 'newname';").Scan(&count)
				Expect(tx.Error).To(BeNil())
				Expect(count).To(Equal(1))
			})

			It("fails to update non-existent user", func() {
				user := model.User{
					ID:        uuid.New(),
					Username:  "nonexistent",
					Email:     "no@acme.com",
					FirstName: "No",
					LastName:  "One",
					Phone:     "+1-555-0001",
					Location:  "na",
				}
				_, err := s.Accounts().UpdateUser(context.TODO(), user)
				Expect(err).To(Equal(store.ErrRecordNotFound))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM users;")
				gormdb.Exec("DELETE FROM organizations;")
			})
		})

		Context("delete", func() {
			It("successfully deletes a user", func() {
				orgID := uuid.New()
				userID := uuid.New()

				tx := gormdb.Exec(fmt.Sprintf(insertOrgStm, orgID, "Test Org", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertUserStm, userID, "deleteuser", "del@acme.com", "Del", "User", "+1-555-0001", "na", "", "", orgID))
				Expect(tx.Error).To(BeNil())

				err := s.Accounts().DeleteUser(context.TODO(), userID)
				Expect(err).To(BeNil())

				var count int
				tx = gormdb.Raw("SELECT COUNT(*) FROM users;").Scan(&count)
				Expect(tx.Error).To(BeNil())
				Expect(count).To(Equal(0))
			})

			It("does not fail when deleting non-existent user", func() {
				err := s.Accounts().DeleteUser(context.TODO(), uuid.New())
				Expect(err).To(BeNil())
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM users;")
				gormdb.Exec("DELETE FROM organizations;")
			})
		})
	})
})
