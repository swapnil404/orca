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
	UpdateOrganization(context.Context, string, string, string) (store.Organization, error)
	DeleteOrganization(context.Context, string, string) error
	GetOrganizationByID(context.Context, string, string) (store.Organization, error)
	ListOrganizationsForUser(context.Context, string) ([]store.Organization, error)
	ListMembersForOrganization(context.Context, string, string) ([]store.OrganizationMembership, error)
	ListProjectsForOrganization(context.Context, string, string) ([]store.Project, error)
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
	mux.HandleFunc("PUT /orgs/{organizationID}", h.updateOrganization)
	mux.HandleFunc("DELETE /orgs/{organizationID}", h.deleteOrganization)
	mux.HandleFunc("GET /orgs/{organizationID}/members", h.listMembers)
	mux.HandleFunc("GET /orgs/{organizationID}/projects", h.listProjects)
}

func (h *OrganizationHandler) updateOrganization(w http.ResponseWriter, r *http.Request) {
	userID, organizationID, ok := h.requireOwner(w, r)
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
	organization, err := h.store.UpdateOrganization(r.Context(), userID, organizationID, request.Name)
	if err != nil {
		h.writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, organization)
}

func (h *OrganizationHandler) deleteOrganization(w http.ResponseWriter, r *http.Request) {
	userID, organizationID, ok := h.requireOwner(w, r)
	if !ok {
		return
	}
	if err := h.store.DeleteOrganization(r.Context(), userID, organizationID); err != nil {
		h.writeMutationError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
	userID, organizationID, ok := organizationRequestIDs(w, r)
	if !ok {
		return
	}
	organization, err := h.store.GetOrganizationByID(r.Context(), userID, organizationID)
	if err != nil {
		h.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, organization)
}

func (h *OrganizationHandler) listMembers(w http.ResponseWriter, r *http.Request) {
	userID, organizationID, ok := organizationRequestIDs(w, r)
	if !ok {
		return
	}
	members, err := h.store.ListMembersForOrganization(r.Context(), userID, organizationID)
	if err != nil {
		h.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, members)
}

func (h *OrganizationHandler) listProjects(w http.ResponseWriter, r *http.Request) {
	userID, organizationID, ok := organizationRequestIDs(w, r)
	if !ok {
		return
	}
	projects, err := h.store.ListProjectsForOrganization(r.Context(), userID, organizationID)
	if err != nil {
		h.writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, projects)
}

func organizationRequestIDs(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return "", "", false
	}
	organizationID := r.PathValue("organizationID")
	if !validUUID(organizationID) {
		writeError(w, http.StatusBadRequest, "invalid organization ID")
		return "", "", false
	}
	return userID, organizationID, true
}

func (h *OrganizationHandler) requireOwner(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return "", "", false
	}
	organizationID := r.PathValue("organizationID")
	if !validUUID(organizationID) {
		writeError(w, http.StatusBadRequest, "invalid organization ID")
		return "", "", false
	}
	membership, err := h.store.GetMembershipForUserAndOrg(r.Context(), userID, organizationID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusForbidden, "organization membership required")
		return "", "", false
	}
	if err != nil {
		writeStoreError(w, err)
		return "", "", false
	}
	if membership.Role != store.OrganizationRoleOwner {
		writeError(w, http.StatusForbidden, "organization owner role required")
		return "", "", false
	}
	return userID, organizationID, true
}

func (h *OrganizationHandler) writeMutationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrOrganizationOwnerRequired):
		writeError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, store.ErrOrganizationHasProjects):
		writeError(w, http.StatusConflict, err.Error())
	default:
		writeStoreError(w, err)
	}
}

func (h *OrganizationHandler) writeReadError(w http.ResponseWriter, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusForbidden, "organization membership required")
		return
	}
	writeStoreError(w, err)
}

func validUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	compact := strings.ReplaceAll(value, "-", "")
	_, err := hex.DecodeString(compact)
	return err == nil
}
