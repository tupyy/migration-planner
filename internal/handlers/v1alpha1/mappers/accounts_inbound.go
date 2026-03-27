package mappers

import (
	"github.com/google/uuid"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/store/model"
)

func OrganizationCreateToModel(req api.OrganizationCreate) model.Organization {
	org := model.Organization{
		ID:          uuid.New(),
		Name:        req.Name,
		Description: req.Description,
		Kind:        string(req.Kind),
		Company:     req.Company,
	}

	return org
}

func OrganizationUpdateToModel(req api.OrganizationUpdate, existing model.Organization) model.Organization {
	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.Description != nil {
		existing.Description = *req.Description
	}
	if req.Company != nil {
		existing.Company = *req.Company
	}

	return existing
}

func UserCreateToModel(req api.UserCreate) model.User {
	user := model.User{
		Username:       req.Username,
		Email:          string(req.Email),
		FirstName:      req.FirstName,
		LastName:       req.LastName,
		Phone:          req.Phone,
		Location:       string(req.Location),
		OrganizationID: *req.OrganizationId,
	}

	if req.Title != nil {
		user.Title = *req.Title
	}
	if req.Bio != nil {
		user.Bio = *req.Bio
	}

	return user
}

func UserUpdateToModel(req api.UserUpdate, existing model.User) model.User {
	if req.Email != nil {
		existing.Email = string(*req.Email)
	}
	if req.FirstName != nil {
		existing.FirstName = *req.FirstName
	}
	if req.LastName != nil {
		existing.LastName = *req.LastName
	}
	if req.Phone != nil {
		existing.Phone = *req.Phone
	}
	if req.Location != nil {
		existing.Location = string(*req.Location)
	}
	if req.Title != nil {
		existing.Title = *req.Title
	}
	if req.Bio != nil {
		existing.Bio = *req.Bio
	}

	return existing
}
