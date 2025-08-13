package model

import (
	v1pb "github.com/authzed/authzed-go/proto/authzed/api/v1"
)

type SubjectType int

func (s SubjectType) String() string {
	switch s {
	case User:
		return UserObject
	case Organization:
		return OrgObject
	default:
		return UserObject // default fallback
	}
}

const (
	User SubjectType = iota
	Organization

	OrgObject        string = "org"
	UserObject       string = "user"
	AssessmentObject string = "assessment"
)

type Subject struct {
	Kind SubjectType
	Id   string
}

// NewUserSubject creates a new Subject with User type.
//
// Parameters:
//   - userID: The ID of the user
//
// Returns:
//   - Subject: A subject representing a user
//
// Example:
//
//	userSubject := NewUserSubject("user123")
//	ownershipFn := WithOwnerRelationship("assessment789", userSubject)
func NewUserSubject(userID string) Subject {
	return Subject{
		Kind: User,
		Id:   userID,
	}
}

// NewOrganizationSubject creates a new Subject with Organization type.
//
// Parameters:
//   - organizationID: The ID of the organization
//
// Returns:
//   - Subject: A subject representing an organization
//
// Example:
//
//	orgSubject := NewOrganizationSubject("org456")
//	readerFn := WithReaderRelationship("assessment789", orgSubject)
func NewOrganizationSubject(organizationID string) Subject {
	return Subject{
		Kind: Organization,
		Id:   organizationID,
	}
}

type RelationshipKind int

func (r RelationshipKind) String() string {
	switch r {
	case ReaderRelationshipKind:
		return "reader"
	case OwnerRelationshipKind:
		return "owner"
	case EditorRelationshipKind:
		return "editor"
	case MemberRelationshipKind:
		return "member"
	default:
		return "unknown"
	}
}

const (
	ReaderRelationshipKind RelationshipKind = iota
	OwnerRelationshipKind
	EditorRelationshipKind
	MemberRelationshipKind
)

type Permission int

func (p Permission) String() string {
	switch p {
	case ReadPermission:
		return "read"
	case EditPermission:
		return "edit"
	case SharePermission:
		return "share"
	case DeletePermission:
		return "delete"
	default:
		return "unknown"
	}
}

const (
	ReadPermission Permission = iota
	EditPermission
	SharePermission
	DeletePermission
)

type Resource struct {
	AssessmentID string
	Permissions  []Permission
}

type Relationship struct {
	AssessmentID     string
	RelationshipKind RelationshipKind
	Subject          Subject
}

type Relationships []Relationship

type RelationshipFn func(updates []*v1pb.RelationshipUpdate) []*v1pb.RelationshipUpdate
