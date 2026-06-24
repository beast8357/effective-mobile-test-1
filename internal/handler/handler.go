package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"effective-mobile/internal/model"
	"effective-mobile/internal/repository"
)

const defaultListLimit = 50

type Handler struct {
	repo *repository.SubscriptionRepository
	log  *slog.Logger
}

func New(repo *repository.SubscriptionRepository, log *slog.Logger) *Handler {
	return &Handler{repo: repo, log: log}
}

// Routes mounts all subscription endpoints under /subscriptions.
// The /total route is registered before the dynamic /{id} route so that
// the literal "total" segment is matched before the UUID parser.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Route("/subscriptions", func(r chi.Router) {
		r.Get("/total", h.Total)
		r.Post("/", h.Create)
		r.Get("/", h.List)
		r.Get("/{id}", h.Get)
		r.Put("/{id}", h.Update)
		r.Delete("/{id}", h.Delete)
	})
	return r
}

type subscriptionRequest struct {
	ServiceName string           `json:"service_name"`
	Price       int              `json:"price"`
	UserID      uuid.UUID        `json:"user_id"`
	StartDate   model.MonthYear  `json:"start_date"`
	EndDate     *model.MonthYear `json:"end_date,omitempty"`
}

func (req subscriptionRequest) validate() error {
	if req.ServiceName == "" {
		return errors.New("service_name is required")
	}
	if req.Price < 0 {
		return errors.New("price must be >= 0")
	}
	if req.UserID == uuid.Nil {
		return errors.New("user_id is required")
	}
	if req.StartDate.IsZero() {
		return errors.New("start_date is required")
	}
	if req.EndDate != nil && req.EndDate.Before(req.StartDate) {
		return errors.New("end_date must be >= start_date")
	}
	return nil
}

func (req subscriptionRequest) toModel(id uuid.UUID) *model.Subscription {
	return &model.Subscription{
		ID:          id,
		ServiceName: req.ServiceName,
		Price:       req.Price,
		UserID:      req.UserID,
		StartDate:   req.StartDate,
		EndDate:     req.EndDate,
	}
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	req, ok := h.decode(w, r)
	if !ok {
		return
	}
	s := req.toModel(uuid.Nil)
	if err := h.repo.Create(r.Context(), s); err != nil {
		h.log.Error("create subscription", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	h.log.Info("subscription created", "id", s.ID, "user_id", s.UserID, "service", s.ServiceName)
	writeJSON(w, http.StatusCreated, s)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	s, err := h.repo.GetByID(r.Context(), id)
	if errors.Is(err, repository.ErrNotFound) {
		writeError(w, http.StatusNotFound, "subscription not found")
		return
	}
	if err != nil {
		h.log.Error("get subscription", "err", err, "id", id)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	req, ok := h.decode(w, r)
	if !ok {
		return
	}
	s := req.toModel(id)
	if err := h.repo.Update(r.Context(), s); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeError(w, http.StatusNotFound, "subscription not found")
			return
		}
		h.log.Error("update subscription", "err", err, "id", id)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	h.log.Info("subscription updated", "id", id)
	writeJSON(w, http.StatusOK, s)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.repo.Delete(r.Context(), id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeError(w, http.StatusNotFound, "subscription not found")
			return
		}
		h.log.Error("delete subscription", "err", err, "id", id)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	h.log.Info("subscription deleted", "id", id)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := repository.ListFilter{Limit: defaultListLimit}

	if v := q.Get("user_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid user_id")
			return
		}
		f.UserID = &id
	}
	if v := q.Get("service_name"); v != "" {
		f.ServiceName = &v
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 || n > 200 {
			writeError(w, http.StatusBadRequest, "limit must be in range 1..200")
			return
		}
		f.Limit = n
	}
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "offset must be >= 0")
			return
		}
		f.Offset = n
	}

	items, err := h.repo.List(r.Context(), f)
	if err != nil {
		h.log.Error("list subscriptions", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"count": len(items),
	})
}

func (h *Handler) Total(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from, err := model.ParseMonthYear(q.Get("from"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid 'from' (expected MM-YYYY)")
		return
	}
	to, err := model.ParseMonthYear(q.Get("to"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid 'to' (expected MM-YYYY)")
		return
	}
	if to.Before(from) {
		writeError(w, http.StatusBadRequest, "'to' must be >= 'from'")
		return
	}

	f := repository.TotalFilter{From: from.Time(), To: to.Time()}
	if v := q.Get("user_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid user_id")
			return
		}
		f.UserID = &id
	}
	if v := q.Get("service_name"); v != "" {
		f.ServiceName = &v
	}

	total, err := h.repo.Total(r.Context(), f)
	if err != nil {
		h.log.Error("total subscriptions", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"from":  from.String(),
		"to":    to.String(),
		"total": total,
	})
}

func (h *Handler) decode(w http.ResponseWriter, r *http.Request) (subscriptionRequest, bool) {
	var req subscriptionRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return req, false
	}
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return req, false
	}
	return req, true
}

func parseID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id, expected UUID")
		return uuid.Nil, false
	}
	return id, true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
