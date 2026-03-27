package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kubev2v/migration-planner/internal/store/model"
)

type Accounts interface {
	// Organizations
	ListOrganizations(ctx context.Context, filter *OrganizationQueryFilter) (model.OrganizationList, error)
	GetOrganization(ctx context.Context, id uuid.UUID) (model.Organization, error)
	CreateOrganization(ctx context.Context, org model.Organization) (model.Organization, error)
	UpdateOrganization(ctx context.Context, org model.Organization) (model.Organization, error)
	DeleteOrganization(ctx context.Context, id uuid.UUID) error

	// Users
	ListUsers(ctx context.Context, filter *UserQueryFilter) (model.UserList, error)
	GetUser(ctx context.Context, username string) (model.User, error)
	CreateUser(ctx context.Context, user model.User) (model.User, error)
	UpdateUser(ctx context.Context, user model.User) (model.User, error)
	DeleteUser(ctx context.Context, id uuid.UUID) error
}

type AccountsStore struct {
	db *gorm.DB
}

var _ Accounts = (*AccountsStore)(nil)

func NewAccountsStore(db *gorm.DB) Accounts {
	return &AccountsStore{db: db}
}

// Organizations

func (s *AccountsStore) ListOrganizations(ctx context.Context, filter *OrganizationQueryFilter) (model.OrganizationList, error) {
	var orgs model.OrganizationList
	tx := s.getDB(ctx).Model(&orgs).Order("created_at DESC").Preload("Users")

	if filter != nil {
		for _, fn := range filter.QueryFn {
			tx = fn(tx)
		}
	}

	result := tx.Find(&orgs)
	if result.Error != nil {
		return nil, result.Error
	}
	return orgs, nil
}

func (s *AccountsStore) GetOrganization(ctx context.Context, id uuid.UUID) (model.Organization, error) {
	var org model.Organization
	result := s.getDB(ctx).Preload("Users").First(&org, "id = ?", id)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return model.Organization{}, ErrRecordNotFound
		}
		return model.Organization{}, result.Error
	}
	return org, nil
}

func (s *AccountsStore) CreateOrganization(ctx context.Context, org model.Organization) (model.Organization, error) {
	result := s.getDB(ctx).Clauses(clause.Returning{}).Create(&org)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrDuplicatedKey) {
			return model.Organization{}, ErrDuplicateKey
		}
		return model.Organization{}, result.Error
	}
	return org, nil
}

func (s *AccountsStore) UpdateOrganization(ctx context.Context, org model.Organization) (model.Organization, error) {
	if err := s.getDB(ctx).First(&model.Organization{}, "id = ?", org.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.Organization{}, ErrRecordNotFound
		}
		return model.Organization{}, err
	}

	now := time.Now()
	org.UpdatedAt = &now
	if err := s.getDB(ctx).Model(&org).Updates(&org).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return model.Organization{}, ErrDuplicateKey
		}
		return model.Organization{}, err
	}
	return org, nil
}

func (s *AccountsStore) DeleteOrganization(ctx context.Context, id uuid.UUID) error {
	result := s.getDB(ctx).Unscoped().Delete(&model.Organization{}, "id = ?", id.String())
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return result.Error
	}
	return nil
}

// Users

func (s *AccountsStore) ListUsers(ctx context.Context, filter *UserQueryFilter) (model.UserList, error) {
	var users model.UserList
	tx := s.getDB(ctx).Model(&users).Order("created_at DESC").Preload("Organization")

	if filter != nil {
		for _, fn := range filter.QueryFn {
			tx = fn(tx)
		}
	}

	result := tx.Find(&users)
	if result.Error != nil {
		return nil, result.Error
	}
	return users, nil
}

func (s *AccountsStore) GetUser(ctx context.Context, username string) (model.User, error) {
	var user model.User
	result := s.getDB(ctx).Preload("Organization").First(&user, "username = ?", username)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return model.User{}, ErrRecordNotFound
		}
		return model.User{}, result.Error
	}
	return user, nil
}

func (s *AccountsStore) CreateUser(ctx context.Context, user model.User) (model.User, error) {
	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}
	result := s.getDB(ctx).Clauses(clause.Returning{}).Create(&user)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrDuplicatedKey) {
			return model.User{}, ErrDuplicateKey
		}
		return model.User{}, result.Error
	}
	return user, nil
}

func (s *AccountsStore) UpdateUser(ctx context.Context, user model.User) (model.User, error) {
	if err := s.getDB(ctx).First(&model.User{}, "id = ?", user.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.User{}, ErrRecordNotFound
		}
		return model.User{}, err
	}

	now := time.Now()
	user.UpdatedAt = &now
	if err := s.getDB(ctx).Model(&user).Updates(&user).Error; err != nil {
		return model.User{}, err
	}
	return user, nil
}

func (s *AccountsStore) DeleteUser(ctx context.Context, id uuid.UUID) error {
	result := s.getDB(ctx).Unscoped().Delete(&model.User{}, "id = ?", id)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return result.Error
	}
	return nil
}

func (s *AccountsStore) getDB(ctx context.Context) *gorm.DB {
	tx := FromContext(ctx)
	if tx != nil {
		return tx
	}
	return s.db
}
