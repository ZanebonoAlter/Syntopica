package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// ── Legacy reference-role read compatibility ───────────────────────────────
//
// The table and GET endpoints remain for one version so old clients can read
// their data. All writes are explicitly retired: reference roles no longer
// participate in any prompt, and analysis methods are managed separately.

// listReferenceRoles returns all roles (admin listing).
// GET /reference-roles
func (h *EnrichmentHandler) listReferenceRoles(c *gin.Context) {
	list, err := h.repo.ListReferenceRoles(c.Request.Context())
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondOK(c, list)
}

// POST /reference-roles
func (h *EnrichmentHandler) createReferenceRole(c *gin.Context) {
	respondError(c, http.StatusGone, "reference roles are read-only; use /analysis-methods")
}

// getReferenceRole fetches one role.
// GET /reference-roles/:id
func (h *EnrichmentHandler) getReferenceRole(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	role, err := h.repo.GetReferenceRoleByID(c.Request.Context(), id)
	if err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return
	}
	respondOK(c, role)
}

// PUT /reference-roles/:id (including the old enable/disable path).
func (h *EnrichmentHandler) updateReferenceRole(c *gin.Context) {
	respondError(c, http.StatusGone, "reference roles are read-only; use /analysis-methods")
}

// DELETE /reference-roles/:id
func (h *EnrichmentHandler) deleteReferenceRole(c *gin.Context) {
	respondError(c, http.StatusGone, "reference roles are read-only; use /analysis-methods")
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}

// isUniqueViolation matches both Postgres (23505 unique_violation) and the
// SQLite test driver (UNIQUE constraint failed) so repo tests share the path.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) {
		return pgErr.SQLState() == "23505"
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}
