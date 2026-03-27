package mappers_test

import (
	"testing"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/handlers/v1alpha1/mappers"
	"github.com/kubev2v/migration-planner/internal/store/model"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

func TestUserUpdateToModel_DoesNotMutateUsername(t *testing.T) {
	existing := model.User{
		Username:  "original",
		Email:     "old@acme.com",
		FirstName: "Old",
		LastName:  "Name",
	}

	newEmail := openapi_types.Email("new@acme.com")
	newFirst := "New"
	req := api.UserUpdate{
		Email:     &newEmail,
		FirstName: &newFirst,
	}

	result := mappers.UserUpdateToModel(req, existing)

	if result.Username != "original" {
		t.Errorf("username was mutated: got %q, want %q", result.Username, "original")
	}
	if result.Email != "new@acme.com" {
		t.Errorf("email not updated: got %q, want %q", result.Email, "new@acme.com")
	}
	if result.FirstName != "New" {
		t.Errorf("firstName not updated: got %q, want %q", result.FirstName, "New")
	}
}

func TestUserUpdateToModel_UpdatesEditableFields(t *testing.T) {
	existing := model.User{
		Username:  "testuser",
		Email:     "old@acme.com",
		FirstName: "Old",
		LastName:  "Last",
		Phone:     "+1-555-0001",
		Location:  "na",
		Title:     "old title",
		Bio:       "old bio",
	}

	newEmail := openapi_types.Email("new@acme.com")
	newFirst := "New"
	newLast := "NewLast"
	newPhone := "+1-555-9999"
	newLoc := api.UserUpdateLocation("emea")
	newTitle := "new title"
	newBio := "new bio"

	req := api.UserUpdate{
		Email:     &newEmail,
		FirstName: &newFirst,
		LastName:  &newLast,
		Phone:     &newPhone,
		Location:  &newLoc,
		Title:     &newTitle,
		Bio:       &newBio,
	}

	result := mappers.UserUpdateToModel(req, existing)

	if result.Username != "testuser" {
		t.Errorf("username mutated: got %q", result.Username)
	}
	if result.Email != "new@acme.com" {
		t.Errorf("email: got %q", result.Email)
	}
	if result.FirstName != "New" {
		t.Errorf("firstName: got %q", result.FirstName)
	}
	if result.LastName != "NewLast" {
		t.Errorf("lastName: got %q", result.LastName)
	}
	if result.Phone != "+1-555-9999" {
		t.Errorf("phone: got %q", result.Phone)
	}
	if result.Location != "emea" {
		t.Errorf("location: got %q", result.Location)
	}
	if result.Title != "new title" {
		t.Errorf("title: got %q", result.Title)
	}
	if result.Bio != "new bio" {
		t.Errorf("bio: got %q", result.Bio)
	}
}

func TestUserUpdateToModel_PartialUpdate(t *testing.T) {
	existing := model.User{
		Username:  "testuser",
		Email:     "old@acme.com",
		FirstName: "Old",
		LastName:  "Last",
		Phone:     "+1-555-0001",
		Location:  "na",
	}

	newLast := "NewLast"
	req := api.UserUpdate{
		LastName: &newLast,
	}

	result := mappers.UserUpdateToModel(req, existing)

	if result.Username != "testuser" {
		t.Errorf("username mutated: got %q", result.Username)
	}
	if result.Email != "old@acme.com" {
		t.Errorf("email changed: got %q", result.Email)
	}
	if result.FirstName != "Old" {
		t.Errorf("firstName changed: got %q", result.FirstName)
	}
	if result.LastName != "NewLast" {
		t.Errorf("lastName not updated: got %q", result.LastName)
	}
	if result.Phone != "+1-555-0001" {
		t.Errorf("phone changed: got %q", result.Phone)
	}
	if result.Location != "na" {
		t.Errorf("location changed: got %q", result.Location)
	}
}
