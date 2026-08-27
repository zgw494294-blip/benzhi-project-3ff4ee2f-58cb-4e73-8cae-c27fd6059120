package httpui

import (
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"strconv"

	"strata-proof/internal/application"
	"strata-proof/internal/domain"
)

//go:embed web/*
var webFiles embed.FS

type Handler struct {
	service *application.Service
	assets  http.Handler
}

func NewHandler(service *application.Service) *Handler {
	root, _ := fs.Sub(webFiles, "web")
	return &Handler{service: service, assets: http.FileServer(http.FS(root))}
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", h.RedirectHome)
	mux.HandleFunc("GET /workbench", h.Workbench)
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", h.assets))
	mux.HandleFunc("GET /api/v1/dossiers", h.ListDossiers)
	mux.HandleFunc("POST /api/v1/dossiers", h.CreateDossier)
	mux.HandleFunc("GET /api/v1/dossiers/{id}", h.GetDossier)
	mux.HandleFunc("PATCH /api/v1/dossiers/{id}", h.UpdateDossier)
	mux.HandleFunc("POST /api/v1/dossiers/{id}/units", h.CreateUnit)
	mux.HandleFunc("POST /api/v1/dossiers/{id}/units/batch", h.CreateUnitsBatch)
	mux.HandleFunc("PUT /api/v1/dossiers/{id}/units/{unitID}", h.UpdateUnit)
	mux.HandleFunc("GET /api/v1/dossiers/{id}/units/{unitID}/revisions", h.UnitHistory)
	mux.HandleFunc("POST /api/v1/dossiers/{id}/relations", h.CreateRelation)
	mux.HandleFunc("PUT /api/v1/dossiers/{id}/relations/{relationID}", h.UpdateRelation)
	mux.HandleFunc("GET /api/v1/dossiers/{id}/relations/{relationID}/revisions", h.RelationHistory)
	mux.HandleFunc("GET /api/v1/dossiers/{id}/relation-path", h.TraceRelationPath)
	mux.HandleFunc("GET /api/v1/dossiers/{id}/audit", h.AuditPage)
	mux.HandleFunc("POST /api/v1/dossiers/{id}/checks", h.RunCheck)
	mux.HandleFunc("GET /api/v1/dossiers/{id}/checks", h.CheckBatches)
	mux.HandleFunc("PATCH /api/v1/dossiers/{id}/findings/{findingID}", h.ResolveFinding)
	mux.HandleFunc("PATCH /api/v1/dossiers/{id}/remediations/{remediationID}", h.ResolveRemediation)
	mux.HandleFunc("POST /api/v1/dossiers/{id}/submit", h.SubmitReview)
	mux.HandleFunc("POST /api/v1/dossiers/{id}/review", h.ReviewDossier)
	mux.HandleFunc("POST /api/v1/dossiers/{id}/credentials", h.IssueCredential)
	mux.HandleFunc("GET /api/v1/credentials/{credentialID}", h.VerifyCredential)
	mux.HandleFunc("POST /api/v1/dossiers/{id}/credentials/{credentialID}/revoke", h.RevokeCredential)
	mux.HandleFunc("POST /api/v1/dossiers/{id}/credentials/{credentialID}/reissue", h.ReissueCredential)
	return operationalMiddleware(securityHeaders(mux))
}

func (h *Handler) ResolveRemediation(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		ExpectedVersion int64  `json:"expectedVersion"`
		Actor           string `json:"actor"`
		IdempotencyKey  string `json:"idempotencyKey"`
		ResolutionNote  string `json:"resolutionNote"`
	}
	if !decode(w, r, &payload) {
		return
	}
	cmd := application.ResolveRemediationCommand{VersionedCommand: application.VersionedCommand{DossierID: r.PathValue("id"), ExpectedVersion: payload.ExpectedVersion, Actor: payload.Actor, IdempotencyKey: payload.IdempotencyKey}, RemediationID: r.PathValue("remediationID"), ResolutionNote: payload.ResolutionNote}
	result, err := h.service.ResolveRemediation(r.Context(), cmd)
	respond(w, result, err, http.StatusOK)
}

func (h *Handler) CreateUnitsBatch(w http.ResponseWriter, r *http.Request) {
	var command application.BatchPutUnitsCommand
	if !decodeLimit(w, r, &command, 256<<10) {
		return
	}
	command.DossierID = r.PathValue("id")
	result, err := h.service.PutUnitsBatch(r.Context(), command)
	respond(w, result, err, http.StatusCreated)
}

func (h *Handler) TraceRelationPath(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.TraceRelationPath(r.Context(), r.PathValue("id"), r.URL.Query().Get("sourceUnitId"), r.URL.Query().Get("targetUnitId"))
	respond(w, result, err, http.StatusOK)
}

func (h *Handler) CheckBatches(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.CheckBatches(r.Context(), r.PathValue("id"), r.URL.Query().Get("severity"), r.URL.Query().Get("changeType"))
	respond(w, result, err, http.StatusOK)
}

func (h *Handler) RedirectHome(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/workbench", http.StatusSeeOther)
}

func (h *Handler) Workbench(w http.ResponseWriter, r *http.Request) {
	data, err := webFiles.ReadFile("web/index.html")
	if err != nil {
		http.Error(w, "工作台资源不可用", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

func (h *Handler) ListDossiers(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	items, err := h.service.ListDossiers(r.Context(), limit, offset)
	respond(w, items, err, http.StatusOK)
}

func (h *Handler) CreateDossier(w http.ResponseWriter, r *http.Request) {
	var command application.CreateDossierCommand
	if !decode(w, r, &command) {
		return
	}
	result, err := h.service.CreateDossier(r.Context(), command)
	respond(w, result, err, http.StatusCreated)
}

func (h *Handler) GetDossier(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.GetDossier(r.Context(), r.PathValue("id"))
	respond(w, result, err, http.StatusOK)
}

func (h *Handler) UpdateDossier(w http.ResponseWriter, r *http.Request) {
	var command application.UpdateDossierCommand
	if !decode(w, r, &command) {
		return
	}
	command.DossierID = r.PathValue("id")
	result, err := h.service.UpdateDossier(r.Context(), command)
	respond(w, result, err, http.StatusOK)
}

func (h *Handler) CreateUnit(w http.ResponseWriter, r *http.Request) {
	h.putUnit(w, r, "", http.StatusCreated)
}
func (h *Handler) UpdateUnit(w http.ResponseWriter, r *http.Request) {
	h.putUnit(w, r, r.PathValue("unitID"), http.StatusOK)
}
func (h *Handler) putUnit(w http.ResponseWriter, r *http.Request, unitID string, status int) {
	var command application.PutUnitCommand
	if !decode(w, r, &command) {
		return
	}
	command.DossierID, command.UnitID = r.PathValue("id"), unitID
	result, err := h.service.PutUnit(r.Context(), command)
	respond(w, result, err, status)
}

func (h *Handler) CreateRelation(w http.ResponseWriter, r *http.Request) {
	h.putRelation(w, r, "", http.StatusCreated)
}
func (h *Handler) UpdateRelation(w http.ResponseWriter, r *http.Request) {
	h.putRelation(w, r, r.PathValue("relationID"), http.StatusOK)
}
func (h *Handler) putRelation(w http.ResponseWriter, r *http.Request, relationID string, status int) {
	var command application.PutRelationCommand
	if !decode(w, r, &command) {
		return
	}
	command.DossierID, command.RelationID = r.PathValue("id"), relationID
	result, err := h.service.PutRelation(r.Context(), command)
	respond(w, result, err, status)
}

func (h *Handler) RunCheck(w http.ResponseWriter, r *http.Request) {
	var command application.VersionedCommand
	if !decode(w, r, &command) {
		return
	}
	command.DossierID = r.PathValue("id")
	result, err := h.service.RunCheck(r.Context(), command)
	respond(w, result, err, http.StatusOK)
}

func (h *Handler) ResolveFinding(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		ExpectedVersion int64  `json:"expectedVersion"`
		Actor           string `json:"actor"`
		IdempotencyKey  string `json:"idempotencyKey"`
		ResolutionNote  string `json:"resolutionNote"`
	}
	if !decode(w, r, &payload) {
		return
	}
	command := application.ResolveFindingCommand{VersionedCommand: application.VersionedCommand{DossierID: r.PathValue("id"), ExpectedVersion: payload.ExpectedVersion, Actor: payload.Actor, IdempotencyKey: payload.IdempotencyKey}, FindingID: r.PathValue("findingID"), ResolutionNote: payload.ResolutionNote}
	result, err := h.service.ResolveFinding(r.Context(), command)
	respond(w, result, err, http.StatusOK)
}

func (h *Handler) SubmitReview(w http.ResponseWriter, r *http.Request) {
	var command application.VersionedCommand
	if !decode(w, r, &command) {
		return
	}
	command.DossierID = r.PathValue("id")
	result, err := h.service.SubmitReview(r.Context(), command)
	respond(w, result, err, http.StatusOK)
}

func (h *Handler) ReviewDossier(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		ExpectedVersion int64                      `json:"expectedVersion"`
		Actor           string                     `json:"actor"`
		IdempotencyKey  string                     `json:"idempotencyKey"`
		Approved        bool                       `json:"approved"`
		Note            string                     `json:"note"`
		Reviewer        string                     `json:"reviewer"`
		Checklist       domain.ReviewChecklist     `json:"checklist"`
		Targets         []application.ReviewTarget `json:"targets"`
	}
	if !decode(w, r, &payload) {
		return
	}
	command := application.ReviewCommand{VersionedCommand: application.VersionedCommand{DossierID: r.PathValue("id"), ExpectedVersion: payload.ExpectedVersion, Actor: payload.Actor, IdempotencyKey: payload.IdempotencyKey}, Approved: payload.Approved, Note: payload.Note, Reviewer: payload.Reviewer, Checklist: payload.Checklist, Targets: payload.Targets}
	result, err := h.service.Review(r.Context(), command)
	respond(w, result, err, http.StatusOK)
}

func (h *Handler) RevokeCredential(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		ExpectedVersion int64  `json:"expectedVersion"`
		Actor           string `json:"actor"`
		IdempotencyKey  string `json:"idempotencyKey"`
		Reason          string `json:"reason"`
	}
	if !decode(w, r, &payload) {
		return
	}
	cmd := application.RevokeCredentialCommand{VersionedCommand: application.VersionedCommand{DossierID: r.PathValue("id"), ExpectedVersion: payload.ExpectedVersion, Actor: payload.Actor, IdempotencyKey: payload.IdempotencyKey}, CredentialID: r.PathValue("credentialID"), Reason: payload.Reason}
	result, err := h.service.RevokeCredential(r.Context(), cmd)
	respond(w, result, err, http.StatusOK)
}

func (h *Handler) ReissueCredential(w http.ResponseWriter, r *http.Request) {
	var command application.VersionedCommand
	if !decode(w, r, &command) {
		return
	}
	command.DossierID = r.PathValue("id")
	result, err := h.service.ReissueCredential(r.Context(), application.ReissueCredentialCommand{VersionedCommand: command, CredentialID: r.PathValue("credentialID")})
	respond(w, result, err, http.StatusCreated)
}

func (h *Handler) IssueCredential(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		ExpectedVersion int64  `json:"expectedVersion"`
		Actor           string `json:"actor"`
		IdempotencyKey  string `json:"idempotencyKey"`
		IssuedBy        string `json:"issuedBy"`
	}
	if !decode(w, r, &payload) {
		return
	}
	command := application.IssueCommand{VersionedCommand: application.VersionedCommand{DossierID: r.PathValue("id"), ExpectedVersion: payload.ExpectedVersion, Actor: payload.Actor, IdempotencyKey: payload.IdempotencyKey}, IssuedBy: payload.IssuedBy}
	result, err := h.service.IssueCredential(r.Context(), command)
	respond(w, result, err, http.StatusCreated)
}

func (h *Handler) VerifyCredential(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.VerifyCredential(r.Context(), r.PathValue("credentialID"))
	respond(w, result, err, http.StatusOK)
}

func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	return decodeLimit(w, r, target, 1<<20)
}

func decodeLimit(w http.ResponseWriter, r *http.Request, target any, limit int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		problem(w, http.StatusBadRequest, "invalid_json", "请求 JSON 无效："+err.Error(), nil)
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		problem(w, http.StatusBadRequest, "invalid_json", "请求只能包含一个 JSON 对象", nil)
		return false
	}
	return true
}

func respond(w http.ResponseWriter, value any, err error, status int) {
	if err != nil {
		code := domain.ErrorCodeOf(err)
		httpStatus := http.StatusInternalServerError
		switch code {
		case domain.CodeValidation:
			httpStatus = 400
		case domain.CodeNotFound:
			httpStatus = 404
		case domain.CodeConflict:
			httpStatus = 409
		case domain.CodeFrozen, domain.CodeState:
			httpStatus = 422
		}
		problem(w, httpStatus, string(code), err.Error(), domain.ErrorDetails(err))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}

func problem(w http.ResponseWriter, status int, code, detail string, details any) {
	w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
	w.WriteHeader(status)
	payload := map[string]any{"type": "about:blank", "title": http.StatusText(status), "status": status, "code": code, "detail": detail}
	if details != nil {
		payload["details"] = details
	}
	json.NewEncoder(w).Encode(payload)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'")
		next.ServeHTTP(w, r)
	})
}
