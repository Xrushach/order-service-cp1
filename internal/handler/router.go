package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"example.com/order-service/internal/order"
	"github.com/go-chi/chi/v5"
	httpSwagger "github.com/swaggo/http-swagger"
)

type OrderService interface {
	Create(ctx context.Context, input order.CreateOrderInput) (order.Order, error)
	GetByID(ctx context.Context, id int64) (order.Order, error)
	List(ctx context.Context) ([]order.Order, error)
	UpdateStatus(ctx context.Context, id int64, status order.Status) (order.Order, error)
	History(ctx context.Context, id int64) ([]order.StatusChange, error)
}

type Handler struct {
	service OrderService
	logger  *slog.Logger
}

func NewRouter(service OrderService, logger *slog.Logger) http.Handler {
	h := &Handler{service: service, logger: logger}
	router := chi.NewRouter()
	router.Use(recoveryMiddleware(logger))
	router.Use(requestLoggingMiddleware(logger))

	router.Get("/swagger/*", httpSwagger.WrapHandler)
	router.Get("/health", h.health)
	router.Route("/api/v1/orders", func(r chi.Router) {
		r.Post("/", h.createOrder)
		r.Get("/", h.listOrders)
		r.Get("/{id}", h.getOrder)
		r.Patch("/{id}/status", h.updateStatus)
		r.Get("/{id}/history", h.getHistory)
	})

	return router
}

// @Summary Проверка состояния сервиса
// @Description Возвращает статус работоспособности сервиса
// @Tags health
// @Produce json
// @Success 200 {object} map[string]string
// @Router /health [get]
func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// @Summary Создать заказ
// @Description Создает новый заказ, проверяет товары и считает сумму
// @Tags orders
// @Accept  json
// @Produce  json
// @Param   request body order.CreateOrderInput true "данные заказа"
// @Success 201 {object} order.Order
// @Failure 400 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Router /api/v1/orders [post]
func (h *Handler) createOrder(w http.ResponseWriter, r *http.Request) {
	var input order.CreateOrderInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "bad request", "BAD_REQUEST")
		return
	}

	o, err := h.service.Create(r.Context(), input)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, o)
}

// @Summary Получить все заказы
// @Description Возвращает список всех заказов
// @Tags orders
// @Produce  json
// @Success 200 {array} order.Order
// @Failure 500 {object} errorResponse
// @Router /api/v1/orders [get]
func (h *Handler) listOrders(w http.ResponseWriter, r *http.Request) {
	list, err := h.service.List(r.Context())
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// @Summary Получить заказ по ID
// @Description Возвращает один заказ по его номеру
// @Tags orders
// @Produce  json
// @Param id path int64 true "ID заказа"
// @Success 200 {object} order.Order
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Router /api/v1/orders/{id} [get]
func (h *Handler) getOrder(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "bad id", "BAD_ID")
		return
	}

	o, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

// @Summary Сменить статус заказа
// @Description Меняет статус, проверяя, можно ли это сделать
// @Tags orders
// @Accept  json
// @Produce  json
// @Param id path int64 true "ID заказа"
// @Param request body order.UpdateStatusInput true "Новый статус"
// @Success 200 {object} order.Order
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Router /api/v1/orders/{id}/status [patch]
func (h *Handler) updateStatus(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "bad id", "BAD_ID")
		return
	}

	var input order.UpdateStatusInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "bad request", "BAD_REQUEST")
		return
	}

	o, err := h.service.UpdateStatus(r.Context(), id, input.Status)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

// @Summary История статусов
// @Description Показывает все изменения статуса заказа.
// @Tags orders
// @Produce  json
// @Param id path int64 true "ID заказа"
// @Success 200 {array} order.StatusChange
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Router /api/v1/orders/{id}/history [get]
func (h *Handler) getHistory(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "bad id", "BAD_ID")
		return
	}

	history, err := h.service.History(r.Context(), id)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, history)
}

func parseID(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}

func (h *Handler) handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, order.ErrOrderNotFound):
		h.errorResponse(w, http.StatusNotFound, "order not found", "NOT_FOUND")
	case errors.Is(err, order.ErrInvalidOrder):
		h.errorResponse(w, http.StatusBadRequest, "invalid order", "BAD_ORDER")
	case errors.Is(err, order.ErrInvalidStatusTransition):
		h.errorResponse(w, http.StatusConflict, "bad status transition", "BAD_TRANSITION")
	case errors.Is(err, order.ErrInvalidStatus):
		h.errorResponse(w, http.StatusBadRequest, "invalid status", "BAD_STATUS")
	default:
		h.logger.Error("internal error", slog.String("error", err.Error()))
		h.errorResponse(w, http.StatusInternalServerError, "internal error", "INTERNAL_ERROR")
	}
}

type errorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

func (h *Handler) errorResponse(w http.ResponseWriter, status int, msg, code string) {
	writeJSON(w, status, errorResponse{Error: msg, Code: code})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
