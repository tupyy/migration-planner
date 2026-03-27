package v1alpha1

import (
	"context"
	"fmt"
	"strings"

	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/handlers/v1alpha1/mappers"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/pkg/log"
)

// (GET /api/v1/identity)
func (h *ServiceHandler) GetIdentity(ctx context.Context, request server.GetIdentityRequestObject) (server.GetIdentityResponseObject, error) {
	logger := log.NewDebugLogger("accounts_handler").
		WithContext(ctx).
		Operation("get_identity").
		Build()

	authUser := auth.MustHaveUser(ctx)
	logger.Step("extract_user").WithString("username", authUser.Username).Log()

	identity, err := h.accountsSrv.GetIdentity(ctx, authUser)
	if err != nil {
		logger.Error(err).Log()
		return server.GetIdentity500JSONResponse{Message: fmt.Sprintf("failed to get identity: %v", err)}, nil
	}

	logger.Success().WithString("username", identity.Username).Log()
	return server.GetIdentity200JSONResponse(mappers.IdentityToApi(identity)), nil
}

// (GET /api/v1/organizations)
func (h *ServiceHandler) ListOrganizations(ctx context.Context, request server.ListOrganizationsRequestObject) (server.ListOrganizationsResponseObject, error) {
	logger := log.NewDebugLogger("accounts_handler").
		WithContext(ctx).
		Operation("list_organizations").
		Build()

	filter := store.NewOrganizationQueryFilter()
	if request.Params.Kind != nil {
		filter = filter.ByKind(string(*request.Params.Kind))
	}
	if request.Params.Name != nil {
		filter = filter.ByName(*request.Params.Name)
	}
	if request.Params.Company != nil {
		filter = filter.ByCompany(*request.Params.Company)
	}

	orgs, err := h.accountsSrv.ListOrganizations(ctx, filter)
	if err != nil {
		logger.Error(err).Log()
		return server.ListOrganizations500JSONResponse{Message: fmt.Sprintf("failed to list organizations: %v", err)}, nil
	}

	logger.Success().WithInt("count", len(orgs)).Log()
	return server.ListOrganizations200JSONResponse(mappers.OrganizationListToApi(orgs)), nil
}

// (POST /api/v1/organizations)
func (h *ServiceHandler) CreateOrganization(ctx context.Context, request server.CreateOrganizationRequestObject) (server.CreateOrganizationResponseObject, error) {
	logger := log.NewDebugLogger("accounts_handler").
		WithContext(ctx).
		Operation("create_organization").
		Build()

	if request.Body == nil {
		return server.CreateOrganization400JSONResponse{Message: "empty body"}, nil
	}

	org := mappers.OrganizationCreateToModel(*request.Body)

	created, err := h.accountsSrv.CreateOrganization(ctx, org)
	if err != nil {
		switch err.(type) {
		case *service.ErrDuplicateKey:
			return server.CreateOrganization400JSONResponse{Message: "organization already exists"}, nil
		default:
			logger.Error(err).Log()
			return server.CreateOrganization500JSONResponse{Message: fmt.Sprintf("failed to create organization: %v", err)}, nil
		}
	}

	logger.Success().WithString("org_name", created.Name).Log()
	return server.CreateOrganization201JSONResponse(mappers.OrganizationToApi(created)), nil
}

// (GET /api/v1/organizations/{id})
func (h *ServiceHandler) GetOrganization(ctx context.Context, request server.GetOrganizationRequestObject) (server.GetOrganizationResponseObject, error) {
	logger := log.NewDebugLogger("accounts_handler").
		WithContext(ctx).
		Operation("get_organization").
		WithUUID("org_id", request.Id).
		Build()

	org, err := h.accountsSrv.GetOrganization(ctx, request.Id)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.GetOrganization404JSONResponse{Message: "organization not found"}, nil
		default:
			logger.Error(err).Log()
			return server.GetOrganization500JSONResponse{Message: fmt.Sprintf("failed to get organization: %v", err)}, nil
		}
	}

	logger.Success().WithString("org_name", org.Name).Log()
	return server.GetOrganization200JSONResponse(mappers.OrganizationToApi(org)), nil
}

// (PUT /api/v1/organizations/{id})
func (h *ServiceHandler) UpdateOrganization(ctx context.Context, request server.UpdateOrganizationRequestObject) (server.UpdateOrganizationResponseObject, error) {
	logger := log.NewDebugLogger("accounts_handler").
		WithContext(ctx).
		Operation("update_organization").
		WithUUID("org_id", request.Id).
		Build()

	if request.Body == nil {
		return server.UpdateOrganization400JSONResponse{Message: "empty body"}, nil
	}

	if request.Body.Name != nil && strings.TrimSpace(*request.Body.Name) == "" {
		return server.UpdateOrganization400JSONResponse{Message: "name cannot be empty"}, nil
	}
	if request.Body.Company != nil && strings.TrimSpace(*request.Body.Company) == "" {
		return server.UpdateOrganization400JSONResponse{Message: "company cannot be empty"}, nil
	}

	existing, err := h.accountsSrv.GetOrganization(ctx, request.Id)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.UpdateOrganization404JSONResponse{Message: "organization not found"}, nil
		default:
			logger.Error(err).Log()
			return server.UpdateOrganization500JSONResponse{Message: fmt.Sprintf("failed to get organization: %v", err)}, nil
		}
	}

	updated := mappers.OrganizationUpdateToModel(*request.Body, existing)
	result, err := h.accountsSrv.UpdateOrganization(ctx, updated)
	if err != nil {
		switch err.(type) {
		case *service.ErrDuplicateKey:
			return server.UpdateOrganization400JSONResponse{Message: "organization already exists"}, nil
		default:
			logger.Error(err).Log()
			return server.UpdateOrganization500JSONResponse{Message: fmt.Sprintf("failed to update organization: %v", err)}, nil
		}
	}

	logger.Success().WithString("org_name", result.Name).Log()
	return server.UpdateOrganization200JSONResponse(mappers.OrganizationToApi(result)), nil
}

// (DELETE /api/v1/organizations/{id})
func (h *ServiceHandler) DeleteOrganization(ctx context.Context, request server.DeleteOrganizationRequestObject) (server.DeleteOrganizationResponseObject, error) {
	logger := log.NewDebugLogger("accounts_handler").
		WithContext(ctx).
		Operation("delete_organization").
		WithUUID("org_id", request.Id).
		Build()

	org, err := h.accountsSrv.GetOrganization(ctx, request.Id)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.DeleteOrganization404JSONResponse{Message: "organization not found"}, nil
		default:
			logger.Error(err).Log()
			return server.DeleteOrganization500JSONResponse{Message: fmt.Sprintf("failed to get organization: %v", err)}, nil
		}
	}

	if err := h.accountsSrv.DeleteOrganization(ctx, request.Id); err != nil {
		logger.Error(err).Log()
		return server.DeleteOrganization500JSONResponse{Message: fmt.Sprintf("failed to delete organization: %v", err)}, nil
	}

	logger.Success().WithString("org_name", org.Name).Log()
	return server.DeleteOrganization200JSONResponse(mappers.OrganizationToApi(org)), nil
}

// (GET /api/v1/organizations/{id}/users)
func (h *ServiceHandler) ListOrganizationUsers(ctx context.Context, request server.ListOrganizationUsersRequestObject) (server.ListOrganizationUsersResponseObject, error) {
	logger := log.NewDebugLogger("accounts_handler").
		WithContext(ctx).
		Operation("list_organization_users").
		WithUUID("org_id", request.Id).
		Build()

	// Verify org exists
	_, err := h.accountsSrv.GetOrganization(ctx, request.Id)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.ListOrganizationUsers404JSONResponse{Message: "organization not found"}, nil
		default:
			logger.Error(err).Log()
			return server.ListOrganizationUsers500JSONResponse{Message: fmt.Sprintf("failed to get organization: %v", err)}, nil
		}
	}

	filter := store.NewUserQueryFilter().ByOrganizationID(request.Id)
	users, err := h.accountsSrv.ListUsers(ctx, filter)
	if err != nil {
		logger.Error(err).Log()
		return server.ListOrganizationUsers500JSONResponse{Message: fmt.Sprintf("failed to list organization users: %v", err)}, nil
	}

	logger.Success().WithInt("count", len(users)).Log()
	return server.ListOrganizationUsers200JSONResponse(mappers.UserListToApi(users)), nil
}

// (PUT /api/v1/organizations/{id}/users/{username})
func (h *ServiceHandler) AddOrganizationUser(ctx context.Context, request server.AddOrganizationUserRequestObject) (server.AddOrganizationUserResponseObject, error) {
	logger := log.NewDebugLogger("accounts_handler").
		WithContext(ctx).
		Operation("add_organization_user").
		WithUUID("org_id", request.Id).
		WithString("username", request.Username).
		Build()

	err := h.accountsSrv.AddUserToOrganization(ctx, request.Id, request.Username)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.AddOrganizationUser404JSONResponse{Message: err.Error()}, nil
		default:
			logger.Error(err).Log()
			return server.AddOrganizationUser500JSONResponse{Message: fmt.Sprintf("failed to add user to organization: %v", err)}, nil
		}
	}

	logger.Success().Log()
	return server.AddOrganizationUser200Response{}, nil
}

// (DELETE /api/v1/organizations/{id}/users/{username})
func (h *ServiceHandler) RemoveOrganizationUser(ctx context.Context, request server.RemoveOrganizationUserRequestObject) (server.RemoveOrganizationUserResponseObject, error) {
	logger := log.NewDebugLogger("accounts_handler").
		WithContext(ctx).
		Operation("remove_organization_user").
		WithUUID("org_id", request.Id).
		WithString("username", request.Username).
		Build()

	err := h.accountsSrv.RemoveUserFromOrganization(ctx, request.Id, request.Username)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.RemoveOrganizationUser404JSONResponse{Message: err.Error()}, nil
		case *service.ErrMembershipMismatch:
			return server.RemoveOrganizationUser400JSONResponse{Message: err.Error()}, nil
		default:
			logger.Error(err).Log()
			return server.RemoveOrganizationUser500JSONResponse{Message: fmt.Sprintf("failed to remove user from organization: %v", err)}, nil
		}
	}

	logger.Success().Log()
	return server.RemoveOrganizationUser200Response{}, nil
}

// (GET /api/v1/users)
func (h *ServiceHandler) ListUsers(ctx context.Context, request server.ListUsersRequestObject) (server.ListUsersResponseObject, error) {
	logger := log.NewDebugLogger("accounts_handler").
		WithContext(ctx).
		Operation("list_users").
		Build()

	filter := store.NewUserQueryFilter()
	if request.Params.OrganizationId != nil {
		filter = filter.ByOrganizationID(*request.Params.OrganizationId)
	}
	if request.Params.Location != nil {
		filter = filter.ByLocation(string(*request.Params.Location))
	}

	users, err := h.accountsSrv.ListUsers(ctx, filter)
	if err != nil {
		logger.Error(err).Log()
		return server.ListUsers500JSONResponse{Message: fmt.Sprintf("failed to list users: %v", err)}, nil
	}

	logger.Success().WithInt("count", len(users)).Log()
	return server.ListUsers200JSONResponse(mappers.UserListToApi(users)), nil
}

// (POST /api/v1/users)
func (h *ServiceHandler) CreateUser(ctx context.Context, request server.CreateUserRequestObject) (server.CreateUserResponseObject, error) {
	logger := log.NewDebugLogger("accounts_handler").
		WithContext(ctx).
		Operation("create_user").
		Build()

	if request.Body == nil {
		return server.CreateUser400JSONResponse{Message: "empty body"}, nil
	}

	if request.Body.OrganizationId == nil {
		return server.CreateUser400JSONResponse{Message: "organizationId is required"}, nil
	}

	user := mappers.UserCreateToModel(*request.Body)

	created, err := h.accountsSrv.CreateUser(ctx, user)
	if err != nil {
		switch err.(type) {
		case *service.ErrDuplicateKey:
			return server.CreateUser409JSONResponse{Message: "user already exists"}, nil
		default:
			logger.Error(err).Log()
			return server.CreateUser500JSONResponse{Message: fmt.Sprintf("failed to create user: %v", err)}, nil
		}
	}

	logger.Success().WithString("username", created.Username).Log()
	return server.CreateUser201JSONResponse(mappers.UserToApi(created)), nil
}

// (GET /api/v1/users/{username})
func (h *ServiceHandler) GetUser(ctx context.Context, request server.GetUserRequestObject) (server.GetUserResponseObject, error) {
	logger := log.NewDebugLogger("accounts_handler").
		WithContext(ctx).
		Operation("get_user").
		WithString("username", request.Username).
		Build()

	user, err := h.accountsSrv.GetUser(ctx, request.Username)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.GetUser404JSONResponse{Message: "user not found"}, nil
		default:
			logger.Error(err).Log()
			return server.GetUser500JSONResponse{Message: fmt.Sprintf("failed to get user: %v", err)}, nil
		}
	}

	logger.Success().WithString("username", user.Username).Log()
	return server.GetUser200JSONResponse(mappers.UserToApi(user)), nil
}

// (PUT /api/v1/users/{username})
func (h *ServiceHandler) UpdateUser(ctx context.Context, request server.UpdateUserRequestObject) (server.UpdateUserResponseObject, error) {
	logger := log.NewDebugLogger("accounts_handler").
		WithContext(ctx).
		Operation("update_user").
		WithString("username", request.Username).
		Build()

	if request.Body == nil {
		return server.UpdateUser400JSONResponse{Message: "empty body"}, nil
	}

	existing, err := h.accountsSrv.GetUser(ctx, request.Username)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.UpdateUser404JSONResponse{Message: "user not found"}, nil
		default:
			logger.Error(err).Log()
			return server.UpdateUser500JSONResponse{Message: fmt.Sprintf("failed to get user: %v", err)}, nil
		}
	}

	updated := mappers.UserUpdateToModel(*request.Body, existing)
	result, err := h.accountsSrv.UpdateUser(ctx, updated)
	if err != nil {
		logger.Error(err).Log()
		return server.UpdateUser500JSONResponse{Message: fmt.Sprintf("failed to update user: %v", err)}, nil
	}

	logger.Success().WithString("username", result.Username).Log()
	return server.UpdateUser200JSONResponse(mappers.UserToApi(result)), nil
}

// (DELETE /api/v1/users/{username})
func (h *ServiceHandler) DeleteUser(ctx context.Context, request server.DeleteUserRequestObject) (server.DeleteUserResponseObject, error) {
	logger := log.NewDebugLogger("accounts_handler").
		WithContext(ctx).
		Operation("delete_user").
		WithString("username", request.Username).
		Build()

	user, err := h.accountsSrv.GetUser(ctx, request.Username)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.DeleteUser404JSONResponse{Message: "user not found"}, nil
		default:
			logger.Error(err).Log()
			return server.DeleteUser500JSONResponse{Message: fmt.Sprintf("failed to get user: %v", err)}, nil
		}
	}

	if err := h.accountsSrv.DeleteUser(ctx, request.Username); err != nil {
		logger.Error(err).Log()
		return server.DeleteUser500JSONResponse{Message: fmt.Sprintf("failed to delete user: %v", err)}, nil
	}

	logger.Success().WithString("username", user.Username).Log()
	return server.DeleteUser200JSONResponse(mappers.UserToApi(user)), nil
}
