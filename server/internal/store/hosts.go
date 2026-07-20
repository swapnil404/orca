package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/betterorca/betterorca/server/internal/store/sqlcdb"
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
