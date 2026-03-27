package v1alpha1_test

import (
	"context"
	"fmt"
	"reflect"

	"github.com/google/uuid"
	v1alpha1 "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/config"
	handlers "github.com/kubev2v/migration-planner/internal/handlers/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/store"
	openapi_types "github.com/oapi-codegen/runtime/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
)

const (
	insertAccountsHandlerOrgStm  = "INSERT INTO organizations (id, name, description, kind, company, parent_id) VALUES ('%s', '%s', '%s', '%s', '%s', %s);"
	insertAccountsHandlerUserStm = "INSERT INTO users (id, username, email, first_name, last_name, phone, location, title, bio, organization_id) VALUES ('%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s');"
)

var _ = Describe("accounts handler", Ordered, func() {
	var (
		s      store.Store
		gormdb *gorm.DB
		srv    *handlers.ServiceHandler
	)

	BeforeAll(func() {
		cfg, err := config.New()
		Expect(err).To(BeNil())
		db, err := store.InitDB(cfg)
		Expect(err).To(BeNil())

		s = store.NewStore(db)
		gormdb = db
		srv = handlers.NewServiceHandler(nil, nil, nil, nil, nil, service.NewAccountsService(s))
	})

	AfterAll(func() {
		_ = s.Close()
	})

	Context("GetIdentity", func() {
		It("returns identity from JWT when user not in DB", func() {
			authUser := auth.User{
				Username:     "jwtonly",
				Organization: "jwt-org",
			}
			ctx := auth.NewTokenContext(context.TODO(), authUser)

			resp, err := srv.GetIdentity(ctx, server.GetIdentityRequestObject{})
			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.GetIdentity200JSONResponse{})))

			body := resp.(server.GetIdentity200JSONResponse)
			Expect(body.Username).To(Equal("jwtonly"))
			Expect(string(body.Kind)).To(Equal("regular"))
			Expect(body.OrganizationId).To(Equal("jwt-org"))
		})

		It("returns admin identity when user belongs to admin org", func() {
			orgID := uuid.New()
			userID := uuid.New()

			tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerOrgStm, orgID, "Admin Org", "desc", "admin", "Red Hat", "NULL"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertAccountsHandlerUserStm, userID, "adminuser", "admin@rh.com", "Admin", "User", "+1-555-0001", "na", "", "", orgID))
			Expect(tx.Error).To(BeNil())

			authUser := auth.User{Username: "adminuser", Organization: "jwt-org"}
			ctx := auth.NewTokenContext(context.TODO(), authUser)

			resp, err := srv.GetIdentity(ctx, server.GetIdentityRequestObject{})
			Expect(err).To(BeNil())

			body := resp.(server.GetIdentity200JSONResponse)
			Expect(body.Username).To(Equal("adminuser"))
			Expect(string(body.Kind)).To(Equal("admin"))
			Expect(body.OrganizationId).To(Equal(orgID.String()))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE FROM users;")
			gormdb.Exec("DELETE FROM organizations;")
		})
	})

	Context("Organizations", func() {
		Context("ListOrganizations", func() {
			It("returns all organizations", func() {
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerOrgStm, uuid.New(), "Org A", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsHandlerOrgStm, uuid.New(), "Org B", "desc", "partner", "Globex", "NULL"))
				Expect(tx.Error).To(BeNil())

				resp, err := srv.ListOrganizations(context.TODO(), server.ListOrganizationsRequestObject{})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.ListOrganizations200JSONResponse{})))
				Expect(resp.(server.ListOrganizations200JSONResponse)).To(HaveLen(2))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM organizations;")
			})
		})

		Context("CreateOrganization", func() {
			It("creates an organization", func() {
				body := v1alpha1.OrganizationCreate{
					Name:        "New Org",
					Description: "desc",
					Kind:        v1alpha1.OrganizationCreateKindPartner,
					Company:     "Acme",
				}
				resp, err := srv.CreateOrganization(context.TODO(), server.CreateOrganizationRequestObject{Body: &body})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.CreateOrganization201JSONResponse{})))

				created := resp.(server.CreateOrganization201JSONResponse)
				Expect(created.Name).To(Equal("New Org"))
				Expect(created.Company).To(Equal("Acme"))
			})

			It("returns 400 for duplicate company+name", func() {
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerOrgStm, uuid.New(), "Sales", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				body := v1alpha1.OrganizationCreate{
					Name:        "Sales",
					Description: "desc",
					Kind:        v1alpha1.OrganizationCreateKindPartner,
					Company:     "Acme",
				}
				resp, err := srv.CreateOrganization(context.TODO(), server.CreateOrganizationRequestObject{Body: &body})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.CreateOrganization400JSONResponse{})))
			})

			It("allows same name in different company", func() {
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerOrgStm, uuid.New(), "Sales", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				body := v1alpha1.OrganizationCreate{
					Name:        "Sales",
					Description: "desc",
					Kind:        v1alpha1.OrganizationCreateKindPartner,
					Company:     "Globex",
				}
				resp, err := srv.CreateOrganization(context.TODO(), server.CreateOrganizationRequestObject{Body: &body})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.CreateOrganization201JSONResponse{})))
			})

			It("allows different names in same company", func() {
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerOrgStm, uuid.New(), "Sales", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				body := v1alpha1.OrganizationCreate{
					Name:        "Engineering",
					Description: "desc",
					Kind:        v1alpha1.OrganizationCreateKindPartner,
					Company:     "Acme",
				}
				resp, err := srv.CreateOrganization(context.TODO(), server.CreateOrganizationRequestObject{Body: &body})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.CreateOrganization201JSONResponse{})))
			})

			It("returns 400 for nil body", func() {
				resp, err := srv.CreateOrganization(context.TODO(), server.CreateOrganizationRequestObject{Body: nil})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.CreateOrganization400JSONResponse{})))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM organizations;")
			})
		})

		Context("GetOrganization", func() {
			It("returns the organization", func() {
				orgID := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerOrgStm, orgID, "Test Org", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				resp, err := srv.GetOrganization(context.TODO(), server.GetOrganizationRequestObject{Id: orgID})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.GetOrganization200JSONResponse{})))

				body := resp.(server.GetOrganization200JSONResponse)
				Expect(body.Name).To(Equal("Test Org"))
			})

			It("returns 404 for missing org", func() {
				resp, err := srv.GetOrganization(context.TODO(), server.GetOrganizationRequestObject{Id: uuid.New()})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.GetOrganization404JSONResponse{})))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM organizations;")
			})
		})

		Context("UpdateOrganization", func() {
			It("updates the organization", func() {
				orgID := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerOrgStm, orgID, "Old Name", "old desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				newName := "New Name"
				body := v1alpha1.OrganizationUpdate{Name: &newName}
				resp, err := srv.UpdateOrganization(context.TODO(), server.UpdateOrganizationRequestObject{Id: orgID, Body: &body})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.UpdateOrganization200JSONResponse{})))

				updated := resp.(server.UpdateOrganization200JSONResponse)
				Expect(updated.Name).To(Equal("New Name"))
			})

			It("returns 404 for missing org", func() {
				newName := "New Name"
				body := v1alpha1.OrganizationUpdate{Name: &newName}
				resp, err := srv.UpdateOrganization(context.TODO(), server.UpdateOrganizationRequestObject{Id: uuid.New(), Body: &body})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.UpdateOrganization404JSONResponse{})))
			})

			It("updates without company field preserves existing company", func() {
				orgID := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerOrgStm, orgID, "Org", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				newDesc := "updated desc"
				body := v1alpha1.OrganizationUpdate{Description: &newDesc}
				resp, err := srv.UpdateOrganization(context.TODO(), server.UpdateOrganizationRequestObject{Id: orgID, Body: &body})
				Expect(err).To(BeNil())

				updated := resp.(server.UpdateOrganization200JSONResponse)
				Expect(updated.Company).To(Equal("Acme"))
			})

			It("returns 400 for empty company", func() {
				orgID := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerOrgStm, orgID, "Org", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				empty := ""
				body := v1alpha1.OrganizationUpdate{Company: &empty}
				resp, err := srv.UpdateOrganization(context.TODO(), server.UpdateOrganizationRequestObject{Id: orgID, Body: &body})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.UpdateOrganization400JSONResponse{})))
			})

			It("returns 400 for whitespace-only company", func() {
				orgID := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerOrgStm, orgID, "Org", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				spaces := "   "
				body := v1alpha1.OrganizationUpdate{Company: &spaces}
				resp, err := srv.UpdateOrganization(context.TODO(), server.UpdateOrganizationRequestObject{Id: orgID, Body: &body})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.UpdateOrganization400JSONResponse{})))
			})

			It("returns 400 for empty name", func() {
				orgID := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerOrgStm, orgID, "Org", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				empty := ""
				body := v1alpha1.OrganizationUpdate{Name: &empty}
				resp, err := srv.UpdateOrganization(context.TODO(), server.UpdateOrganizationRequestObject{Id: orgID, Body: &body})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.UpdateOrganization400JSONResponse{})))
			})

			It("returns 400 for nil body", func() {
				resp, err := srv.UpdateOrganization(context.TODO(), server.UpdateOrganizationRequestObject{Id: uuid.New(), Body: nil})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.UpdateOrganization400JSONResponse{})))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM organizations;")
			})
		})

		Context("DeleteOrganization", func() {
			It("deletes the organization", func() {
				orgID := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerOrgStm, orgID, "To Delete", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				resp, err := srv.DeleteOrganization(context.TODO(), server.DeleteOrganizationRequestObject{Id: orgID})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.DeleteOrganization200JSONResponse{})))

				deleted := resp.(server.DeleteOrganization200JSONResponse)
				Expect(deleted.Name).To(Equal("To Delete"))
			})

			It("returns 404 for missing org", func() {
				resp, err := srv.DeleteOrganization(context.TODO(), server.DeleteOrganizationRequestObject{Id: uuid.New()})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.DeleteOrganization404JSONResponse{})))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM organizations;")
			})
		})
	})

	Context("Users", func() {
		Context("ListUsers", func() {
			It("returns all users", func() {
				orgID := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerOrgStm, orgID, "Test Org", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsHandlerUserStm, uuid.New(), "user1", "u1@acme.com", "First", "User", "+1-555-0001", "na", "", "", orgID))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsHandlerUserStm, uuid.New(), "user2", "u2@acme.com", "Second", "User", "+1-555-0002", "emea", "", "", orgID))
				Expect(tx.Error).To(BeNil())

				resp, err := srv.ListUsers(context.TODO(), server.ListUsersRequestObject{})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.ListUsers200JSONResponse{})))
				Expect(resp.(server.ListUsers200JSONResponse)).To(HaveLen(2))
			})

			It("filters by organizationId", func() {
				orgA := uuid.New()
				orgB := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerOrgStm, orgA, "Org A", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsHandlerOrgStm, orgB, "Org B", "desc", "partner", "Globex", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsHandlerUserStm, uuid.New(), "usera", "a@acme.com", "A", "User", "+1-555-0001", "na", "", "", orgA))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsHandlerUserStm, uuid.New(), "userb", "b@globex.com", "B", "User", "+1-555-0002", "na", "", "", orgB))
				Expect(tx.Error).To(BeNil())

				resp, err := srv.ListUsers(context.TODO(), server.ListUsersRequestObject{
					Params: v1alpha1.ListUsersParams{OrganizationId: &orgA},
				})
				Expect(err).To(BeNil())
				users := resp.(server.ListUsers200JSONResponse)
				Expect(users).To(HaveLen(1))
				Expect(users[0].Username).To(Equal("usera"))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM users;")
				gormdb.Exec("DELETE FROM organizations;")
			})
		})

		Context("CreateUser", func() {
			It("creates a user", func() {
				orgID := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerOrgStm, orgID, "Test Org", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				body := v1alpha1.UserCreate{
					Username:       "newuser",
					Email:          openapi_types.Email("new@acme.com"),
					FirstName:      "New",
					LastName:       "User",
					Phone:          "+1-555-0001",
					Location:       v1alpha1.UserCreateLocationNa,
					OrganizationId: &orgID,
				}
				resp, err := srv.CreateUser(context.TODO(), server.CreateUserRequestObject{Body: &body})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.CreateUser201JSONResponse{})))

				created := resp.(server.CreateUser201JSONResponse)
				Expect(created.Username).To(Equal("newuser"))
				Expect(created.OrganizationId).To(Equal(orgID))
			})

			It("returns 409 for duplicate username", func() {
				orgID := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerOrgStm, orgID, "Test Org", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsHandlerUserStm, uuid.New(), "dupuser", "dup@acme.com", "Dup", "User", "+1-555-0001", "na", "", "", orgID))
				Expect(tx.Error).To(BeNil())

				body := v1alpha1.UserCreate{
					Username:       "dupuser",
					Email:          openapi_types.Email("dup2@acme.com"),
					FirstName:      "Dup",
					LastName:       "Two",
					Phone:          "+1-555-0002",
					Location:       v1alpha1.UserCreateLocationNa,
					OrganizationId: &orgID,
				}
				resp, err := srv.CreateUser(context.TODO(), server.CreateUserRequestObject{Body: &body})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.CreateUser409JSONResponse{})))
			})

			It("returns 400 when organizationId is nil", func() {
				body := v1alpha1.UserCreate{
					Username:  "noorg",
					Email:     openapi_types.Email("noorg@acme.com"),
					FirstName: "No",
					LastName:  "Org",
					Phone:     "+1-555-0001",
					Location:  v1alpha1.UserCreateLocationNa,
				}
				resp, err := srv.CreateUser(context.TODO(), server.CreateUserRequestObject{Body: &body})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.CreateUser400JSONResponse{})))
			})

			It("returns 400 for nil body", func() {
				resp, err := srv.CreateUser(context.TODO(), server.CreateUserRequestObject{Body: nil})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.CreateUser400JSONResponse{})))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM users;")
				gormdb.Exec("DELETE FROM organizations;")
			})
		})

		Context("GetUser", func() {
			It("returns the user by username", func() {
				orgID := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerOrgStm, orgID, "Test Org", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsHandlerUserStm, uuid.New(), "testuser", "test@acme.com", "Test", "User", "+1-555-0001", "na", "", "", orgID))
				Expect(tx.Error).To(BeNil())

				resp, err := srv.GetUser(context.TODO(), server.GetUserRequestObject{Username: "testuser"})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.GetUser200JSONResponse{})))

				body := resp.(server.GetUser200JSONResponse)
				Expect(body.Username).To(Equal("testuser"))
				Expect(string(body.Email)).To(Equal("test@acme.com"))
			})

			It("returns 404 for missing user", func() {
				resp, err := srv.GetUser(context.TODO(), server.GetUserRequestObject{Username: "ghost"})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.GetUser404JSONResponse{})))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM users;")
				gormdb.Exec("DELETE FROM organizations;")
			})
		})

		Context("UpdateUser", func() {
			It("updates editable fields", func() {
				orgID := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerOrgStm, orgID, "Test Org", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsHandlerUserStm, uuid.New(), "updateme", "old@acme.com", "Old", "Name", "+1-555-0001", "na", "", "", orgID))
				Expect(tx.Error).To(BeNil())

				newEmail := openapi_types.Email("new@acme.com")
				newFirst := "New"
				newLoc := v1alpha1.UserUpdateLocation("emea")
				body := v1alpha1.UserUpdate{
					Email:     &newEmail,
					FirstName: &newFirst,
					Location:  &newLoc,
				}
				resp, err := srv.UpdateUser(context.TODO(), server.UpdateUserRequestObject{Username: "updateme", Body: &body})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.UpdateUser200JSONResponse{})))

				updated := resp.(server.UpdateUser200JSONResponse)
				Expect(updated.Username).To(Equal("updateme"))
				Expect(string(updated.Email)).To(Equal("new@acme.com"))
				Expect(updated.FirstName).To(Equal("New"))
				Expect(string(updated.Location)).To(Equal("emea"))
			})

			It("does not mutate username", func() {
				orgID := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerOrgStm, orgID, "Test Org", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsHandlerUserStm, uuid.New(), "immutable", "imm@acme.com", "Imm", "User", "+1-555-0001", "na", "", "", orgID))
				Expect(tx.Error).To(BeNil())

				newFirst := "Updated"
				body := v1alpha1.UserUpdate{FirstName: &newFirst}
				resp, err := srv.UpdateUser(context.TODO(), server.UpdateUserRequestObject{Username: "immutable", Body: &body})
				Expect(err).To(BeNil())

				updated := resp.(server.UpdateUser200JSONResponse)
				Expect(updated.Username).To(Equal("immutable"))
				Expect(updated.FirstName).To(Equal("Updated"))
			})

			It("returns 404 for missing user", func() {
				newFirst := "New"
				body := v1alpha1.UserUpdate{FirstName: &newFirst}
				resp, err := srv.UpdateUser(context.TODO(), server.UpdateUserRequestObject{Username: "ghost", Body: &body})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.UpdateUser404JSONResponse{})))
			})

			It("returns 400 for nil body", func() {
				resp, err := srv.UpdateUser(context.TODO(), server.UpdateUserRequestObject{Username: "someone", Body: nil})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.UpdateUser400JSONResponse{})))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM users;")
				gormdb.Exec("DELETE FROM organizations;")
			})
		})

		Context("DeleteUser", func() {
			It("deletes the user by username", func() {
				orgID := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerOrgStm, orgID, "Test Org", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsHandlerUserStm, uuid.New(), "todelete", "del@acme.com", "Del", "User", "+1-555-0001", "na", "", "", orgID))
				Expect(tx.Error).To(BeNil())

				resp, err := srv.DeleteUser(context.TODO(), server.DeleteUserRequestObject{Username: "todelete"})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.DeleteUser200JSONResponse{})))

				deleted := resp.(server.DeleteUser200JSONResponse)
				Expect(deleted.Username).To(Equal("todelete"))
			})

			It("returns 404 for missing user", func() {
				resp, err := srv.DeleteUser(context.TODO(), server.DeleteUserRequestObject{Username: "ghost"})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.DeleteUser404JSONResponse{})))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM users;")
				gormdb.Exec("DELETE FROM organizations;")
			})
		})
	})

	Context("Membership", func() {
		Context("ListOrganizationUsers", func() {
			It("returns users belonging to the org", func() {
				orgID := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerOrgStm, orgID, "Test Org", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsHandlerUserStm, uuid.New(), "member1", "m1@acme.com", "M", "One", "+1-555-0001", "na", "", "", orgID))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsHandlerUserStm, uuid.New(), "member2", "m2@acme.com", "M", "Two", "+1-555-0002", "na", "", "", orgID))
				Expect(tx.Error).To(BeNil())

				resp, err := srv.ListOrganizationUsers(context.TODO(), server.ListOrganizationUsersRequestObject{Id: orgID})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.ListOrganizationUsers200JSONResponse{})))
				Expect(resp.(server.ListOrganizationUsers200JSONResponse)).To(HaveLen(2))
			})

			It("returns 404 for missing org", func() {
				resp, err := srv.ListOrganizationUsers(context.TODO(), server.ListOrganizationUsersRequestObject{Id: uuid.New()})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.ListOrganizationUsers404JSONResponse{})))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM users;")
				gormdb.Exec("DELETE FROM organizations;")
			})
		})

		Context("AddOrganizationUser", func() {
			It("moves user to the target org", func() {
				orgA := uuid.New()
				orgB := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerOrgStm, orgA, "Org A", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsHandlerOrgStm, orgB, "Org B", "desc", "partner", "Globex", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsHandlerUserStm, uuid.New(), "mover", "mover@acme.com", "Mover", "User", "+1-555-0001", "na", "", "", orgA))
				Expect(tx.Error).To(BeNil())

				resp, err := srv.AddOrganizationUser(context.TODO(), server.AddOrganizationUserRequestObject{Id: orgB, Username: "mover"})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.AddOrganizationUser200Response{})))

				// Verify user moved
				getResp, err := srv.GetUser(context.TODO(), server.GetUserRequestObject{Username: "mover"})
				Expect(err).To(BeNil())
				user := getResp.(server.GetUser200JSONResponse)
				Expect(user.OrganizationId).To(Equal(orgB))
			})

			It("returns 404 for missing org", func() {
				orgID := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerOrgStm, orgID, "Org", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsHandlerUserStm, uuid.New(), "someuser", "u@acme.com", "Some", "User", "+1-555-0001", "na", "", "", orgID))
				Expect(tx.Error).To(BeNil())

				resp, err := srv.AddOrganizationUser(context.TODO(), server.AddOrganizationUserRequestObject{Id: uuid.New(), Username: "someuser"})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.AddOrganizationUser404JSONResponse{})))
			})

			It("returns 404 for missing user", func() {
				orgID := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerOrgStm, orgID, "Org", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				resp, err := srv.AddOrganizationUser(context.TODO(), server.AddOrganizationUserRequestObject{Id: orgID, Username: "ghost"})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.AddOrganizationUser404JSONResponse{})))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM users;")
				gormdb.Exec("DELETE FROM organizations;")
			})
		})

		Context("RemoveOrganizationUser", func() {
			It("deletes user from the org", func() {
				orgID := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerOrgStm, orgID, "Org", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsHandlerUserStm, uuid.New(), "removeme", "rm@acme.com", "Remove", "Me", "+1-555-0001", "na", "", "", orgID))
				Expect(tx.Error).To(BeNil())

				resp, err := srv.RemoveOrganizationUser(context.TODO(), server.RemoveOrganizationUserRequestObject{Id: orgID, Username: "removeme"})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.RemoveOrganizationUser200Response{})))

				// Verify user is gone
				getResp, err := srv.GetUser(context.TODO(), server.GetUserRequestObject{Username: "removeme"})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(getResp)).To(Equal(reflect.TypeOf(server.GetUser404JSONResponse{})))
			})

			It("returns 400 when user belongs to different org", func() {
				orgA := uuid.New()
				orgB := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerOrgStm, orgA, "Org A", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsHandlerOrgStm, orgB, "Org B", "desc", "partner", "Globex", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsHandlerUserStm, uuid.New(), "wrongorg", "w@acme.com", "Wrong", "Org", "+1-555-0001", "na", "", "", orgB))
				Expect(tx.Error).To(BeNil())

				resp, err := srv.RemoveOrganizationUser(context.TODO(), server.RemoveOrganizationUserRequestObject{Id: orgA, Username: "wrongorg"})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.RemoveOrganizationUser400JSONResponse{})))
			})

			It("returns 404 for missing org", func() {
				orgID := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerOrgStm, orgID, "Org", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAccountsHandlerUserStm, uuid.New(), "someuser", "u@acme.com", "Some", "User", "+1-555-0001", "na", "", "", orgID))
				Expect(tx.Error).To(BeNil())

				resp, err := srv.RemoveOrganizationUser(context.TODO(), server.RemoveOrganizationUserRequestObject{Id: uuid.New(), Username: "someuser"})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.RemoveOrganizationUser404JSONResponse{})))
			})

			It("returns 404 for missing user", func() {
				orgID := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertAccountsHandlerOrgStm, orgID, "Org", "desc", "partner", "Acme", "NULL"))
				Expect(tx.Error).To(BeNil())

				resp, err := srv.RemoveOrganizationUser(context.TODO(), server.RemoveOrganizationUserRequestObject{Id: orgID, Username: "ghost"})
				Expect(err).To(BeNil())
				Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.RemoveOrganizationUser404JSONResponse{})))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE FROM users;")
				gormdb.Exec("DELETE FROM organizations;")
			})
		})
	})
})
