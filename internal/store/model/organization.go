package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Organization struct {
	ID          uuid.UUID `gorm:"primaryKey;column:id;type:VARCHAR(255);"`
	CreatedAt   time.Time `gorm:"not null;default:now()"`
	UpdatedAt   *time.Time
	Name        string `gorm:"uniqueIndex:idx_company_name;not null"`
	Description string
	Kind        string     `gorm:"not null;type:VARCHAR(50)"`
	Icon        []byte     `gorm:"type:bytea"`
	Company     string     `gorm:"uniqueIndex:idx_company_name;not null;type:VARCHAR(200)"`
	ParentID    *uuid.UUID `gorm:"type:VARCHAR(255)"`
	Users       []User     `gorm:"foreignKey:OrganizationID;references:ID;"`
}

type OrganizationList []Organization

func (o Organization) String() string {
	val, _ := json.Marshal(o)
	return string(val)
}
