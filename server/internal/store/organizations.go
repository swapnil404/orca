package store

import (
	"context"
	"strings"
	"time"

	"github.com/swapnil404/orca/server/internal/store/sqlcdb"
)

// OrganizationRole is a member's authorization role in an organization.
type OrganizationRole string

const (
	// OrganizationRoleOwner identifies an organization owner.
	OrganizationRoleOwner OrganizationRole = "owner"
	// OrganizationRoleAdmin identifies an organization administrator.
	OrganizationRoleAdmin OrganizationRole = "admin"
	// OrganizationRoleMember identifies a regular organization member.
	OrganizationRoleMember OrganizationRole = "member"
)

// Organization owns projects and groups users through memberships.
type Organization struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
}

// OrganizationMembership associates a user with an organization and role.
type OrganizationMembership struct {
	ID             string           `json:"id"`
	OrganizationID string           `json:"organization_id"`
	UserID         string           `json:"user_id"`
	Role           OrganizationRole `json:"role"`
	CreatedAt      time.Time        `json:"created_at"`
	Email          string           `json:"email,omitempty"`
}

// CreateOrganization creates an organization and owner membership atomically.
func (s *Postgres) CreateOrganization(ctx context.Context, userID, name string) (Organization, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Organization{}, err
	}
	defer tx.Rollback()
	queries := s.queries.WithTx(tx)

	organization, err := queries.CreateOrganization(ctx, name)
	if err != nil {
		return Organization{}, err
	}
	if _, err := queries.CreateMembership(ctx, sqlcdb.CreateMembershipParams{
		OrganizationID: organization.ID,
		UserID:         userID,
		Role:           sqlcdb.OrganizationRoleOwner,
	}); err != nil {
		return Organization{}, err
	}
	if err := tx.Commit(); err != nil {
		return Organization{}, err
	}
	return organizationFromSQLC(organization), nil
}

// GetOrganizationByID returns an organization by ID.
func (s *Postgres) GetOrganizationByID(ctx context.Context, organizationID string) (Organization, error) {
	organization, err := s.queries.GetOrganizationByID(ctx, organizationID)
	if err != nil {
		return Organization{}, err
	}
	return organizationFromSQLC(organization), nil
}

// GetOrganizationBySlug returns an organization by slug.
func (s *Postgres) GetOrganizationBySlug(ctx context.Context, slug string) (Organization, error) {
	organization, err := s.queries.GetOrganizationBySlug(ctx, slug)
	if err != nil {
		return Organization{}, err
	}
	return organizationFromSQLC(organization), nil
}

// ListOrganizationsForUser returns organizations in which userID is a member.
func (s *Postgres) ListOrganizationsForUser(ctx context.Context, userID string) ([]Organization, error) {
	rows, err := s.queries.ListOrganizationsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	organizations := make([]Organization, 0, len(rows))
	for _, row := range rows {
		organizations = append(organizations, organizationFromSQLC(row))
	}
	return organizations, nil
}

// ListMembersForOrganization returns active members of an organization.
func (s *Postgres) ListMembersForOrganization(ctx context.Context, organizationID string) ([]OrganizationMembership, error) {
	rows, err := s.queries.ListMembersForOrganization(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	members := make([]OrganizationMembership, 0, len(rows))
	for _, row := range rows {
		member := OrganizationMembership{
			ID: row.ID, OrganizationID: row.OrganizationID, UserID: row.UserID,
			Role: OrganizationRole(row.Role), CreatedAt: row.CreatedAt,
		}
		if row.Email.Valid {
			member.Email = row.Email.String
		}
		members = append(members, member)
	}
	return members, nil
}

// ListProjectsForOrganization returns active projects in an organization.
func (s *Postgres) ListProjectsForOrganization(ctx context.Context, organizationID string) ([]Project, error) {
	rows, err := s.queries.ListProjectsForOrganization(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	projects := make([]Project, 0, len(rows))
	for _, row := range rows {
		projects = append(projects, projectFromSQLC(row))
	}
	return projects, nil
}

// GetMembershipForUserAndOrg returns a user's membership in an organization.
func (s *Postgres) GetMembershipForUserAndOrg(ctx context.Context, userID, organizationID string) (OrganizationMembership, error) {
	row, err := s.queries.GetMembershipForUserAndOrg(ctx, sqlcdb.GetMembershipForUserAndOrgParams{
		OrganizationID: organizationID, UserID: userID,
	})
	if err != nil {
		return OrganizationMembership{}, err
	}
	return membershipFromSQLC(row), nil
}

func organizationFromSQLC(organization sqlcdb.Organization) Organization {
	return Organization{
		ID: organization.ID, Name: organization.Name, Slug: organization.Slug, CreatedAt: organization.CreatedAt,
	}
}

func membershipFromSQLC(membership sqlcdb.OrganizationMembership) OrganizationMembership {
	return OrganizationMembership{
		ID: membership.ID, OrganizationID: membership.OrganizationID, UserID: membership.UserID,
		Role: OrganizationRole(membership.Role), CreatedAt: membership.CreatedAt,
	}
}

func createPersonalOrganization(ctx context.Context, queries *sqlcdb.Queries, userID, email string) error {
	organization, err := queries.CreateOrganization(ctx, personalOrganizationName(email))
	if err != nil {
		return err
	}
	_, err = queries.CreateMembership(ctx, sqlcdb.CreateMembershipParams{
		OrganizationID: organization.ID, UserID: userID, Role: sqlcdb.OrganizationRoleOwner,
	})
	return err
}

func personalOrganizationName(email string) string {
	localPart := email
	if at := strings.IndexByte(email, '@'); at >= 0 {
		localPart = email[:at]
	}
	if localPart == "" {
		return "personal workspace"
	}
	return localPart + "'s workspace"
}
