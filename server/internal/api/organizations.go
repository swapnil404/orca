package api

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"

	"github.com/swapnil404/orca/server/internal/store"
)

type organizationStore interface {
	CreateOrganization(context.Context, string, string) (store.Organization, error)
	GetOrganizationByID(context.Context, string) (store.Organization, error)
	ListOrganizationsForUser(context.Context, string) ([]store.Organization, error)
	ListMembersForOrganization(context.Context, string) ([]store.OrganizationMembership, error)
	ListProjectsForOrganization(context.Context, string) ([]store.Project, error)
	GetMembershipForUserAndOrg(context.Context, string, string) (store.OrganizationMembership, error)
}

// OrganizationHandler serves organization and membership endpoints.
type OrganizationHandler struct {
	store organizationStore
}

// NewOrganizationHandler creates the organization API handler.
func NewOrganizationHandler(organizations organizationStore) *OrganizationHandler {
	return &OrganizationHandler{store: organizations}
}

// RegisterRoutes registers authenticated organization routes on mux.
func (h *OrganizationHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /orgs", h.listOrganizations)
	mux.HandleFunc("POST /orgs", h.createOrganization)
	mux.HandleFunc("GET /orgs/{organizationID}", h.getOrganization)
	mux.HandleFunc("GET /orgs/{organizationID}/members", h.listMembers)
	mux.HandleFunc("GET /orgs/{organizationID}/projects", h.listProjects)
}

func (h *OrganizationHandler) listOrganizations(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	organizations, err := h.store.ListOrganizationsForUser(r.Context(), userID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, organizations)
}

func (h *OrganizationHandler) createOrganization(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	var request struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	request.Name = strings.TrimSpace(request.Name)
	if request.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	organization, err := h.store.CreateOrganization(r.Context(), userID, request.Name)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, organization)
}

func (h *OrganizationHandler) getOrganization(w http.ResponseWriter, r *http.Request) {
	organizationID, ok := h.requireMembership(w, r)
	if !ok {
		return
	}
	organization, err := h.store.GetOrganizationByID(r.Context(), organizationID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, organization)
}

func (h *OrganizationHandler) listMembers(w http.ResponseWriter, r *http.Request) {
	organizationID, ok := h.requireMembership(w, r)
	if !ok {
		return
	}
	members, err := h.store.ListMembersForOrganization(r.Context(), organizationID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, members)
}

func (h *OrganizationHandler) listProjects(w http.ResponseWriter, r *http.Request) {
	organizationID, ok := h.requireMembership(w, r)
	if !ok {
		return
	}
	projects, err := h.store.ListProjectsForOrganization(r.Context(), organizationID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, projects)
}

func (h *OrganizationHandler) requireMembership(w http.ResponseWriter, r *http.Request) (string, bool) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return "", false
	}
	organizationID := r.PathValue("organizationID")
	if !validUUID(organizationID) {
		writeError(w, http.StatusBadRequest, "invalid organization ID")
		return "", false
	}
	if _, err := h.store.GetMembershipForUserAndOrg(r.Context(), userID, organizationID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusForbidden, "organization membership required")
			return "", false
		}
		writeStoreError(w, err)
		return "", false
	}
	return organizationID, true
}

func validUUID(value string) bool {
	compact := strings.ReplaceAll(value, "-", "")
	if len(compact) != 32 {
		return false
	}
	_, err := hex.DecodeString(compact)
	return err == nil
}
