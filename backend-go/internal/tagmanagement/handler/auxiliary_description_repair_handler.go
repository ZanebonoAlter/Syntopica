package handler

// Auxiliary-label description repair (board-health-triage-advice seam B2 /
// task 2.2) — the three HTTP resources of the shared-description repair chain:
//
//	GET  /api/auxiliary-labels/:id/description-repair-context   → live label +
//	      opaque version token + directly referencing boards + active tag count +
//	      latest task (pure read, creates nothing)
//	POST /api/auxiliary-labels/:id/description-repairs          → create one
//	      persistent pending job (202; no embedding / matching work yet)
//	GET  /api/auxiliary-labels/:id/description-repairs/:job_id  → the persisted
//	      job, ownership-checked against the label (pure read)
//
// The handler only binds parameters, maps errors and hands the DTO through:
// every rule (the three description rules, the opaque source version, the
// idempotency key, the per-label mutex) lives in the storage layer — this file
// must never grow a second copy of a formula.
//
// The service is injected so the routes can be exercised without touching
// production wiring: this seam does NOT register the routes in
// tagmanagement/routes.go (that is task 2.6) and starts no goroutine.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/tagmanagement/repository"
	"syntopica-backend/internal/tagmanagement/service"
)

// auxiliaryDescriptionRepairHandler serves the description-repair endpoints.
type auxiliaryDescriptionRepairHandler struct {
	svc *service.AuxRepairService
}

// RegisterAuxiliaryDescriptionRepairRoutes registers the four description-
// repair resources under the existing /auxiliary-labels group. The ":id"
// wildcard name matches the group's existing routes (Gin shares one node per
// wildcard name), and ":job_id" is the new child wildcard.
//
// svc is injected (not constructed here) so the caller decides the database
// and the tests can drive the routes against a testcontainer.
func RegisterAuxiliaryDescriptionRepairRoutes(rg *gin.RouterGroup, svc *service.AuxRepairService) {
	h := &auxiliaryDescriptionRepairHandler{svc: svc}
	auxiliary := rg.Group("/auxiliary-labels")
	{
		auxiliary.GET("/:id/description-repair-context", h.getRepairContext)
		auxiliary.POST("/:id/description-repairs", h.createRepairJob)
		auxiliary.GET("/:id/description-repairs/:job_id", h.getRepairJob)
		auxiliary.POST("/:id/description-repairs/:job_id/retry", h.retryRepairJob)
	}
}

// getRepairContext serves GET .../description-repair-context — the live state
// the user confirms against (spec「核对并确认共享影响」). Pure read: it never
// creates a task, and it never exposes vectors or the private claim columns.
func (h *auxiliaryDescriptionRepairHandler) getRepairContext(c *gin.Context) {
	if h.svc == nil {
		respondRepairInternalError(c, "description-repair-context",
			fmt.Errorf("description-repair service is not configured"))
		return
	}
	id, ok := parsePositiveBigIntParam(c, "id")
	if !ok {
		return
	}
	ctx, err := h.svc.GetRepairContext(c.Request.Context(), uint(id))
	if err != nil {
		respondRepairMapped(c, "description-repair-context", err)
		return
	}
	respondOK(c, ctx)
}

// createRepairJob serves POST .../description-repairs — one confirmed
// description rewrite becomes a persisted pending job (202). The chain's
// later stages (embedding / atomic save / matching) are NOT executed here:
// this seam only records the confirmed request.
//
// Idempotency is decided by the STORED request inside the storage layer, so a
// retry of the same key + payload returns the same job — even after the
// source label was deleted or disabled. The handler therefore never looks the
// label up before creating: such a pre-check would kill the legitimate replay.
func (h *auxiliaryDescriptionRepairHandler) createRepairJob(c *gin.Context) {
	if h.svc == nil {
		respondRepairInternalError(c, "description-repairs",
			fmt.Errorf("description-repair service is not configured"))
		return
	}
	id, ok := parsePositiveBigIntParam(c, "id")
	if !ok {
		return
	}
	var req service.AuxRepairCreateRequest
	if err := decodeSingleJSONObject(c.Request.Body, &req); err != nil {
		respondRepairError(c, http.StatusBadRequest, "invalid request body")
		return
	}
	job, err := h.svc.CreateJob(c.Request.Context(), uint(id), req)
	if err != nil {
		respondRepairMapped(c, "description-repairs", err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"success": true, "data": job})
}

// getRepairJob serves GET .../description-repairs/:job_id — the persisted
// task, ownership-checked: a job id that belongs to another label is not
// found (no cross-label leak). Historical jobs stay readable after the
// source label was deleted (the repair tables carry no FK by design).
func (h *auxiliaryDescriptionRepairHandler) getRepairJob(c *gin.Context) {
	if h.svc == nil {
		respondRepairInternalError(c, "description-repair-job",
			fmt.Errorf("description-repair service is not configured"))
		return
	}
	id, ok := parsePositiveBigIntParam(c, "id")
	if !ok {
		return
	}
	jobID, ok := parsePositiveBigIntParam(c, "job_id")
	if !ok {
		return
	}
	job, err := h.svc.GetJob(c.Request.Context(), uint(id), uint(jobID))
	if err != nil {
		respondRepairMapped(c, "description-repair-job", err)
		return
	}
	respondOK(c, job)
}

// parsePositiveBigIntParam parses a path id for the description-repair
// endpoints. The label / job id columns are PostgreSQL signed bigints, so a
// value above MaxInt64 is a bad request (400) — it must never reach the
// driver, which would refuse the encoding and answer 500 (B2 review, Low).
// A syntactically valid but nonexistent id (e.g. MaxInt64) passes here and is
// answered 404 by the service.
//
// The shared parseUintParam (used by the older endpoints) is deliberately NOT
// changed — this restriction applies to the three new repair endpoints only.
func parsePositiveBigIntParam(c *gin.Context, name string) (uint64, bool) {
	parsed, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || parsed == 0 {
		respondError(c, http.StatusBadRequest, fmt.Errorf("invalid %s", name))
		return 0, false
	}
	if parsed > math.MaxInt64 {
		respondError(c, http.StatusBadRequest, fmt.Errorf("invalid %s", name))
		return 0, false
	}
	return parsed, true
}

// decodeSingleJSONObject reads EXACTLY ONE JSON object from the body.
//
// Gin's ShouldBindJSON decodes once and ignores the rest of the stream, so a
// body like `{"description":…}{"x":1}` or `{"description":…} garbage` was
// accepted and still created a repair task (B2 review, Medium). This helper
// requires the second decode to hit io.EOF: a second object or trailing
// garbage is a malformed request. Trailing WHITESPACE stays legal.
//
// Only this endpoint uses it — the global Gin binding behaviour is unchanged.
func decodeSingleJSONObject(body io.Reader, target any) error {
	dec := json.NewDecoder(body)
	if err := dec.Decode(target); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("request body must contain exactly one JSON object")
		}
		return err
	}
	return nil
}

// retryRepairJob serves POST .../description-repairs/:job_id/retry — the
// EXPLICIT retry of a failed or interrupted task.
//
// The endpoint takes NO parameters: the body may be empty or an empty JSON
// object; a malformed body, a second JSON object or ANY parameter is a bad
// request (the user must not believe a parameter was applied).
//
// The handler only binds the parameters and calls the thin service: it never
// runs a phase and never spawns a worker — the task is re-queued in the
// database and picked up by the running executor (or by the next scan).
func (h *auxiliaryDescriptionRepairHandler) retryRepairJob(c *gin.Context) {
	if h.svc == nil {
		respondRepairInternalError(c, "description-repair-retry",
			fmt.Errorf("description-repair service is not configured"))
		return
	}
	id, ok := parsePositiveBigIntParam(c, "id")
	if !ok {
		return
	}
	jobID, ok := parsePositiveBigIntParam(c, "job_id")
	if !ok {
		return
	}
	if err := validateEmptyRetryBody(c.Request.Body); err != nil {
		respondRepairError(c, http.StatusBadRequest, "invalid request body")
		return
	}
	job, err := h.svc.RetryJob(c.Request.Context(), uint(id), uint(jobID))
	if err != nil {
		respondRepairMapped(c, "description-repair-retry", err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"success": true, "data": job})
}

// validateEmptyRetryBody accepts an empty body or exactly one EMPTY JSON
// object. A malformed body, a second object, trailing garbage or any
// parameter is an error: the retry endpoint changes nothing about the
// request, and the user must not think otherwise.
func validateEmptyRetryBody(body io.Reader) error {
	dec := json.NewDecoder(body)
	var raw map[string]json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		if errors.Is(err, io.EOF) {
			return nil // an empty body is allowed
		}
		return err
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("the retry body must contain at most one JSON object")
		}
		return err
	}
	if raw == nil {
		// A JSON `null` body decodes into a nil map — it is NOT an empty body.
		// The contract allows an empty body or an empty object only.
		return fmt.Errorf("the retry body must be an empty JSON object")
	}
	if len(raw) > 0 {
		return fmt.Errorf("the retry endpoint takes no parameters")
	}
	return nil
}

// respondRepairMapped maps a storage/service error onto the contract's HTTP
// status. Known 4xx carry our own wording; anything else is an internal
// error whose raw DB detail must not reach the client (it is logged only).
func respondRepairMapped(c *gin.Context, op string, err error) {
	status, message := repairErrorStatus(err)
	if status == 0 {
		respondRepairInternalError(c, op, err)
		return
	}
	respondRepairError(c, status, message)
}

// repairErrorStatus maps the description-repair sentinels onto 4xx. It
// returns status 0 for "not a client error" — the caller then answers 500.
// The description rules, the version guard, the idempotency conflict and the
// per-label mutex all come from the storage layer (single source); this table
// only chooses the status code.
func repairErrorStatus(err error) (int, string) {
	switch {
	case errors.Is(err, repository.ErrRepairLabelNotFound):
		return http.StatusNotFound, "auxiliary label not found"
	case errors.Is(err, repository.ErrRepairLabelNotActive):
		return http.StatusConflict, "auxiliary label is disabled"
	case errors.Is(err, repository.ErrRepairVersionConflict):
		return http.StatusConflict, "auxiliary label has changed, reload and confirm again"
	case errors.Is(err, repository.ErrRepairIdempotencyConflict):
		return http.StatusConflict, "idempotency key was already used with a different request"
	case errors.Is(err, repository.ErrRepairJobAlreadyActive):
		// The wrapped detail (job id + status) is ours, not a DB error.
		return http.StatusConflict, err.Error()
	case errors.Is(err, repository.ErrRepairIdempotencyKeyEmpty):
		return http.StatusBadRequest, "idempotency_key is required"
	case errors.Is(err, repository.ErrRepairIdempotencyKeyTooLong):
		return http.StatusBadRequest, "idempotency_key must be at most 64 characters"
	case errors.Is(err, repository.ErrRepairExpectedVersionInvalid):
		return http.StatusBadRequest, "expected_version must be a non-empty string of at most 64 characters"
	case errors.Is(err, repository.ErrRepairDescriptionEmpty):
		return http.StatusBadRequest, "description must not be empty"
	case errors.Is(err, repository.ErrRepairDescriptionTooLong):
		return http.StatusBadRequest, "description must be at most 500 characters"
	case errors.Is(err, repository.ErrRepairDescriptionRepeatsLabel):
		return http.StatusBadRequest, "description must not only repeat the label"
	case errors.Is(err, gorm.ErrRecordNotFound):
		return http.StatusNotFound, "repair job not found"
	case errors.Is(err, service.AuxRepairRetryNotAllowed):
		return http.StatusConflict, "repair task cannot be retried in its current state"
	default:
		return 0, ""
	}
}

// respondRepairError answers with the project's error envelope.
func respondRepairError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"success": false, "error": message})
}

// respondRepairInternalError answers 500 without leaking the raw error (which
// may carry DB detail); the cause is logged server-side.
func respondRepairInternalError(c *gin.Context, op string, err error) {
	logging.Errorf("[auxiliary-description-repair] %s failed: %v", op, err)
	respondRepairError(c, http.StatusInternalServerError, "internal server error")
}
