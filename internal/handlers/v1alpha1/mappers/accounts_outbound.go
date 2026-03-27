package mappers

import (
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/store/model"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

func OrganizationToApi(org model.Organization) api.Organization {
	result := api.Organization{
		Id:        org.ID,
		Name:      org.Name,
		Kind:      api.OrganizationKind(org.Kind),
		CreatedAt: org.CreatedAt,
	}

	if org.Description != "" {
		result.Description = &org.Description
	}
	result.Company = org.Company
	if org.UpdatedAt != nil {
		result.UpdatedAt = *org.UpdatedAt
	}

	return result
}

func OrganizationListToApi(orgs model.OrganizationList) api.OrganizationList {
	result := make(api.OrganizationList, len(orgs))
	for i, org := range orgs {
		result[i] = OrganizationToApi(org)
	}
	return result
}

func UserToApi(user model.User) api.User {
	result := api.User{
		Username:  user.Username,
		Email:     openapi_types.Email(user.Email),
		FirstName: user.FirstName,
		LastName:  user.LastName,
		Phone:     user.Phone,
		Location:  api.UserLocation(user.Location),
		CreatedAt: user.CreatedAt,
	}

	if user.Title != "" {
		result.Title = &user.Title
	}
	if user.Bio != "" {
		result.Bio = &user.Bio
	}
	result.OrganizationId = user.OrganizationID
	if user.UpdatedAt != nil {
		result.UpdatedAt = user.UpdatedAt
	}

	return result
}

func UserListToApi(users model.UserList) api.UserList {
	result := make(api.UserList, len(users))
	for i, user := range users {
		result[i] = UserToApi(user)
	}
	return result
}

func IdentityToApi(identity service.Identity) api.Identity {
	result := api.Identity{
		Username:       identity.Username,
		Kind:           api.IdentityKind(identity.Kind),
		OrganizationId: identity.OrganizationID,
	}
	return result
}
