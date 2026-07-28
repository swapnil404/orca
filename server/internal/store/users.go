package store

import (
	"context"
	"database/sql"
	"errors"
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
	DeletedAt    sql.NullTime
}

// UserIDForOAuthIdentity returns an existing identity's user or atomically creates an OAuth-only user.
func (s *Postgres) UserIDForOAuthIdentity(ctx context.Context, provider, providerUserID, providerEmail, newUserID string) (string, error) {
	params := sqlcdb.GetOAuthIdentityUserIDParams{Provider: provider, ProviderUserID: providerUserID}
	userID, err := s.queries.GetOAuthIdentityUserID(ctx, params)
	if err == nil {
		return userID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	queries := s.queries.WithTx(tx)
	if _, err := queries.CreateOAuthUser(ctx, newUserID); err != nil {
		return "", err
	}
	_, err = queries.CreateOAuthIdentity(ctx, sqlcdb.CreateOAuthIdentityParams{
		Provider: provider, ProviderUserID: providerUserID, ProviderEmail: providerEmail, UserID: newUserID,
	})
	if err == nil {
		if err := tx.Commit(); err != nil {
			return "", err
		}
		return newUserID, nil
	}

	// A concurrent callback may have inserted this identity first. Roll back the
	// unused user and return the winner's mapping when it is now visible.
	if rollbackErr := tx.Rollback(); rollbackErr != nil {
		return "", errors.Join(err, rollbackErr)
	}
	if existingUserID, lookupErr := s.queries.GetOAuthIdentityUserID(ctx, params); lookupErr == nil {
		return existingUserID, nil
	}
	return "", err
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

// UserIsActive reports whether userID identifies a user that has not been deleted.
func (s *Postgres) UserIsActive(ctx context.Context, userID string) (bool, error) {
	return s.queries.UserIsActive(ctx, userID)
}

// SoftDeleteUser marks an active user as deleted.
func (s *Postgres) SoftDeleteUser(ctx context.Context, userID string) error {
	_, err := s.queries.SoftDeleteUser(ctx, userID)
	return err
}

func userFromSQLC(user sqlcdb.User) User {
	return User{
		ID: user.ID, Email: user.Email, PasswordHash: user.PasswordHash,
		CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt, DeletedAt: user.DeletedAt,
	}
}
