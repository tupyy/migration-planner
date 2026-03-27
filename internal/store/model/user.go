package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID             uuid.UUID `gorm:"primaryKey;column:id;type:VARCHAR(255);"`
	CreatedAt      time.Time `gorm:"not null;default:now()"`
	UpdatedAt      *time.Time
	Username       string        `gorm:"uniqueIndex;not null;type:VARCHAR(255)"`
	Email          string        `gorm:"not null;type:VARCHAR(255)"`
	FirstName      string        `gorm:"not null;type:VARCHAR(100)"`
	LastName       string        `gorm:"not null;type:VARCHAR(100)"`
	Phone          string        `gorm:"not null;type:VARCHAR(50)"`
	Location       string        `gorm:"not null;type:VARCHAR(10)"`
	Title          string        `gorm:"type:VARCHAR(200)"`
	Bio            string        `gorm:"type:TEXT"`
	OrganizationID uuid.UUID     `gorm:"not null;type:VARCHAR(255)"`
	Organization   *Organization `gorm:"foreignKey:OrganizationID" json:"-"`
}

type UserList []User

func (u User) String() string {
	val, _ := json.Marshal(u)
	return string(val)
}
