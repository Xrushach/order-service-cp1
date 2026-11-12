package service

import (
	"context"
	"strings"
	"time"

	"example.com/order-service/internal/order"
)

type OrderService struct {
	repository order.Repository
}

func NewOrderService(repository order.Repository) *OrderService {
	return &OrderService{repository: repository}
}

func (s *OrderService) Create(ctx context.Context, input order.CreateOrderInput) (order.Order, error) {
	if len(input.Items) == 0 {
		return order.Order{}, order.ErrInvalidOrder
	}
	var totalPrice int64
	for _, item := range input.Items {
		if strings.TrimSpace(item.Name) == "" || item.Quantity <= 0 || item.Price <= 0 {
			return order.Order{}, order.ErrInvalidOrder
		}
		totalPrice += item.Price * int64(item.Quantity)
	}

	now := time.Now().UTC()
	newOrder := order.Order{
		Items:      input.Items,
		TotalPrice: totalPrice,
		Status:     order.StatusNew,
		CreatedAt:  now,
		History:    []order.StatusChange{{Status: order.StatusNew, ChangedAt: now}},
	}
	return s.repository.Create(ctx, newOrder)
}

func (s *OrderService) GetByID(ctx context.Context, id int64) (order.Order, error) {
	return s.repository.GetByID(ctx, id)
}

func (s *OrderService) List(ctx context.Context) ([]order.Order, error) {
	return s.repository.List(ctx)
}

func (s *OrderService) UpdateStatus(ctx context.Context, id int64, status order.Status) (order.Order, error) {
	currOrder, err := s.repository.GetByID(ctx, id)
	if err != nil {
		return order.Order{}, err
	}

	if !transIsGood(currOrder.Status, status) {
		return order.Order{}, order.ErrInvalidStatusTransition
	}
	currOrder.Status = status
	currOrder.History = append(currOrder.History, order.StatusChange{
		Status:    status,
		ChangedAt: time.Now().UTC(),
	})

	return s.repository.Update(ctx, currOrder)
}

func (s *OrderService) History(ctx context.Context, id int64) ([]order.StatusChange, error) {
	o, err := s.repository.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return o.History, nil
}

func transIsGood(from, to order.Status) bool {
	switch from {
	case order.StatusNew:
		return to == order.StatusProcessing || to == order.StatusCancelled
	case order.StatusProcessing:
		return to == order.StatusCompleted || to == order.StatusCancelled
	default:
		return false
	}
}
