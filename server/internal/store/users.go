package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/swapnil404/orca/server/internal/store/sqlcdb"
)

// User is an Orca control-plane account.
type User struct {
	ID           string
	Email        sql.NullString
	PasswordHash sql.NullString
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// CreatePasswordUser persists an account with an email/password credential.
func (s *Postgres) CreatePasswordUser(ctx context.Context, id, email, passwordHash string) (User, error) {
	user, err := s.queries.CreatePasswordUser(ctx, sqlcdb.CreatePasswordUserParams{
		ID: id, Lower: email, PasswordHash: sql.NullString{String: passwordHash, Valid: true},
	})
	if err != nil {
		return User{}, err
	}
	return userFromSQLC(user), nil
}

// UserByEmail returns the account with a case-insensitively matching email.
func (s *Postgres) UserByEmail(ctx context.Context, email string) (User, error) {
	user, err := s.queries.GetUserByEmail(ctx, email)
	if err != nil {
		return User{}, err
	}
	return userFromSQLC(user), nil
}

func userFromSQLC(user sqlcdb.User) User {
	return User{
		ID: user.ID, Email: user.Email, PasswordHash: user.PasswordHash,
		CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt,
	}
}
