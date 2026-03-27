package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
)

type AccountsService struct {
	store store.Store
}

func NewAccountsService(store store.Store) *AccountsService {
	return &AccountsService{store: store}
}

// Identity

type Identity struct {
	Username       string
	Kind           string
	OrganizationID string
}

// GetIdentity resolves the authenticated user's identity for bootstrap.
// If a local account exists, kind and orgID come from the backend.
// If not, kind defaults to "regular" and orgID comes from the JWT.
func (s *AccountsService) GetIdentity(ctx context.Context, authUser auth.User) (Identity, error) {
	user, err := s.store.Accounts().GetUser(ctx, authUser.Username)
	if err != nil {
		if !errors.Is(err, store.ErrRecordNotFound) {
			return Identity{}, err
		}
		// user not found so return a regular user inferred from jwt
		return Identity{
			Username:       authUser.Username,
			Kind:           "regular",
			OrganizationID: authUser.Organization,
		}, nil
	}

	identity := Identity{
		Username:       user.Username,
		Kind:           "regular",
		OrganizationID: user.OrganizationID.String(),
	}

	if user.Organization != nil {
		switch user.Organization.Kind {
		case "admin":
			identity.Kind = "admin"
		case "partner":
			identity.Kind = "partner"
		}
	}

	return identity, nil
}

// Organizations

func (s *AccountsService) ListOrganizations(ctx context.Context, filter *store.OrganizationQueryFilter) (model.OrganizationList, error) {
	return s.store.Accounts().ListOrganizations(ctx, filter)
}

func (s *AccountsService) GetOrganization(ctx context.Context, id uuid.UUID) (model.Organization, error) {
	org, err := s.store.Accounts().GetOrganization(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return model.Organization{}, NewErrResourceNotFound(id, "organization")
		}
		return model.Organization{}, err
	}
	return org, nil
}

func (s *AccountsService) CreateOrganization(ctx context.Context, org model.Organization) (model.Organization, error) {
	created, err := s.store.Accounts().CreateOrganization(ctx, org)
	if err != nil {
		if errors.Is(err, store.ErrDuplicateKey) {
			return model.Organization{}, NewErrDuplicateKey("organization", fmt.Sprintf("%s/%s", org.Company, org.Name))
		}
		return model.Organization{}, err
	}
	return created, nil
}

func (s *AccountsService) UpdateOrganization(ctx context.Context, org model.Organization) (model.Organization, error) {
	result, err := s.store.Accounts().UpdateOrganization(ctx, org)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return model.Organization{}, NewErrResourceNotFound(org.ID, "organization")
		}
		if errors.Is(err, store.ErrDuplicateKey) {
			return model.Organization{}, NewErrDuplicateKey("organization", fmt.Sprintf("%s/%s", org.Company, org.Name))
		}
		return model.Organization{}, err
	}
	return result, nil
}

func (s *AccountsService) DeleteOrganization(ctx context.Context, id uuid.UUID) error {
	return s.store.Accounts().DeleteOrganization(ctx, id)
}

// Users

func (s *AccountsService) ListUsers(ctx context.Context, filter *store.UserQueryFilter) (model.UserList, error) {
	return s.store.Accounts().ListUsers(ctx, filter)
}

func (s *AccountsService) GetUser(ctx context.Context, username string) (model.User, error) {
	user, err := s.store.Accounts().GetUser(ctx, username)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return model.User{}, NewErrUserNotFound(username)
		}
		return model.User{}, err
	}
	return user, nil
}

func (s *AccountsService) CreateUser(ctx context.Context, user model.User) (model.User, error) {
	created, err := s.store.Accounts().CreateUser(ctx, user)
	if err != nil {
		if errors.Is(err, store.ErrDuplicateKey) {
			return model.User{}, NewErrDuplicateKey("user", user.Username)
		}
		return model.User{}, err
	}
	return created, nil
}

func (s *AccountsService) UpdateUser(ctx context.Context, user model.User) (model.User, error) {
	result, err := s.store.Accounts().UpdateUser(ctx, user)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return model.User{}, NewErrUserNotFound(user.Username)
		}
		return model.User{}, err
	}
	return result, nil
}

func (s *AccountsService) DeleteUser(ctx context.Context, username string) error {
	user, err := s.GetUser(ctx, username)
	if err != nil {
		return err
	}
	return s.store.Accounts().DeleteUser(ctx, user.ID)
}

// AddUserToOrganization moves the user to a different organization.
func (s *AccountsService) AddUserToOrganization(ctx context.Context, orgID uuid.UUID, username string) error {
	if _, err := s.GetOrganization(ctx, orgID); err != nil {
		return err
	}

	user, err := s.GetUser(ctx, username)
	if err != nil {
		return err
	}

	user.Organization = nil
	user.OrganizationID = orgID
	_, err = s.store.Accounts().UpdateUser(ctx, user)
	return err
}

// RemoveUserFromOrganization removes a user from the specified organization.
// Since a backend user must always belong to an organization, this deletes the user.
func (s *AccountsService) RemoveUserFromOrganization(ctx context.Context, orgID uuid.UUID, username string) error {
	if _, err := s.GetOrganization(ctx, orgID); err != nil {
		return err
	}

	user, err := s.GetUser(ctx, username)
	if err != nil {
		return err
	}

	if user.OrganizationID != orgID {
		return NewErrMembershipMismatch(username, orgID)
	}

	return s.store.Accounts().DeleteUser(ctx, user.ID)
}
