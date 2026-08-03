package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/swapnil404/orca/server/internal/store/sqlcdb"
)

// HostStatus describes whether an agent has connected to a registered host.
type HostStatus string

const (
	// HostStatusNeverConnected means no agent has connected for this host.
	HostStatusNeverConnected HostStatus = "never_connected"
	// HostStatusOnline means the host currently has an active agent session.
	HostStatusOnline HostStatus = "online"
	// HostStatusOffline means an agent connected previously but is now disconnected.
	HostStatusOffline HostStatus = "offline"
)

// Host is a registered machine that may run an Orca agent.
type Host struct {
	ID             string
	UserID         string
	TokenHash      []byte
	TokenExpiresAt time.Time
	Status         HostStatus
	CreatedAt      time.Time
	ConnectedAt    sql.NullTime
}

// CreateHostParams contains the persistent values for a new host.
type CreateHostParams struct {
	ID             string
	UserID         string
	TokenHash      []byte
	TokenExpiresAt time.Time
	Status         HostStatus
}

// Postgres stores host records in the server metadata database.
type Postgres struct {
	db      *sql.DB
	queries *sqlcdb.Queries
}

// NewPostgres creates a host store backed by db.
func NewPostgres(db *sql.DB) *Postgres {
	return &Postgres{db: db, queries: sqlcdb.New(db)}
}

// CreateHost persists a new host record.
func (s *Postgres) CreateHost(ctx context.Context, params CreateHostParams) (Host, error) {
	host, err := s.queries.CreateHost(ctx, sqlcdb.CreateHostParams{
		ID:             params.ID,
		UserID:         params.UserID,
		TokenHash:      params.TokenHash,
		TokenExpiresAt: params.TokenExpiresAt,
		Status:         string(params.Status),
	})
	if err != nil {
		return Host{}, err
	}
	return hostFromSQLC(host), nil
}

// HostByTokenHash returns the host whose stored token hash matches tokenHash.
func (s *Postgres) HostByTokenHash(ctx context.Context, tokenHash []byte) (Host, error) {
	host, err := s.queries.GetHostByTokenHash(ctx, tokenHash)
	if err != nil {
		return Host{}, err
	}
	return hostFromSQLC(host), nil
}

// UpdateHostStatus changes the connection status of a host.
func (s *Postgres) UpdateHostStatus(ctx context.Context, hostID string, status HostStatus) error {
	return s.queries.UpdateHostStatus(ctx, sqlcdb.UpdateHostStatusParams{
		ID:     hostID,
		Status: string(status),
	})
}

// GetHost returns a host by ID.
func (s *Postgres) GetHost(ctx context.Context, hostID string) (Host, error) {
	host, err := s.queries.GetHost(ctx, hostID)
	if err != nil {
		return Host{}, err
	}
	return hostFromSQLC(host), nil
}

// RotateHostToken replaces the connection token for an owned host.
func (s *Postgres) RotateHostToken(ctx context.Context, hostID, userID string, tokenHash []byte, expiresAt time.Time) (HostStatus, bool, error) {
	status, err := s.queries.RotateHostToken(ctx, sqlcdb.RotateHostTokenParams{
		ID:             hostID,
		UserID:         userID,
		TokenHash:      tokenHash,
		TokenExpiresAt: expiresAt,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return HostStatus(status), err == nil, err
}

// RevokeUnusedHostToken expires the connection token for an owned host that has never connected.
func (s *Postgres) RevokeUnusedHostToken(ctx context.Context, hostID, userID string, tokenHash []byte, expiresAt time.Time) (bool, error) {
	count, err := s.queries.RevokeUnusedHostToken(ctx, sqlcdb.RevokeUnusedHostTokenParams{
		ID:             hostID,
		UserID:         userID,
		TokenHash:      tokenHash,
		TokenExpiresAt: expiresAt,
	})
	return count > 0, err
}

// DeleteUnusedHost removes an owned host when no cluster references it.
func (s *Postgres) DeleteUnusedHost(ctx context.Context, hostID, userID string) (bool, error) {
	count, err := s.queries.DeleteUnusedHost(ctx, sqlcdb.DeleteUnusedHostParams{ID: hostID, UserID: userID})
	return count > 0, err
}

func hostFromSQLC(host sqlcdb.Host) Host {
	return Host{
		ID:             host.ID,
		UserID:         host.UserID,
		TokenHash:      host.TokenHash,
		TokenExpiresAt: host.TokenExpiresAt,
		Status:         HostStatus(host.Status),
		CreatedAt:      host.CreatedAt,
		ConnectedAt:    host.ConnectedAt,
	}
}
