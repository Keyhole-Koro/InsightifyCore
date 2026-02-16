package entity

import "strings"

const DemoUserID UserID = "demo-user"

// UserID identifies a logical user boundary in gateway services.
type UserID string

// User is a lightweight entity boundary for user-scoped operations.
type User struct {
	ID UserID
}

func NewUser(rawID string) User {
	return User{ID: NormalizeUserID(rawID)}
}

func NormalizeUserID(raw string) UserID {
	return UserID(strings.TrimSpace(raw))
}

func (id UserID) String() string {
	return strings.TrimSpace(string(id))
}

func (id UserID) IsZero() bool {
	return id.String() == ""
}
