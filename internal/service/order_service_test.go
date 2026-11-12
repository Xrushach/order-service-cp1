package service_test

import (
	"context"
	"testing"

	"example.com/order-service/internal/order"
	"example.com/order-service/internal/repository/memory"
	"example.com/order-service/internal/service"
	"github.com/stretchr/testify/require"
)

func makeItems() []order.Item {
	return []order.Item{
		{Name: "Kola", Quantity: 1, Price: 100},
		{Name: "Chipsi", Quantity: 2, Price: 150},
	}
}

func TestCreateOrder(t *testing.T) {
	cases := []struct {
		name      string
		input     order.CreateOrderInput
		wantErr   error
		wantTotal int64
	}{
		{name: "success", input: order.CreateOrderInput{Items: makeItems()}, wantErr: nil, wantTotal: 100 + 2*150},
		{name: "empty", input: order.CreateOrderInput{Items: []order.Item{}}, wantErr: order.ErrInvalidOrder},
		{name: "empty name", input: order.CreateOrderInput{Items: []order.Item{{Name: " ", Quantity: 1, Price: 100}}}, wantErr: order.ErrInvalidOrder},
		{name: "zero quantity", input: order.CreateOrderInput{Items: []order.Item{{Name: "Item", Quantity: 0, Price: 100}}}, wantErr: order.ErrInvalidOrder},
		{name: "negative quantity", input: order.CreateOrderInput{Items: []order.Item{{Name: "Item", Quantity: -1, Price: 100}}}, wantErr: order.ErrInvalidOrder},
		{name: "zero price", input: order.CreateOrderInput{Items: []order.Item{{Name: "Item", Quantity: 1, Price: 0}}}, wantErr: order.ErrInvalidOrder},
		{name: "negative price", input: order.CreateOrderInput{Items: []order.Item{{Name: "Item", Quantity: 1, Price: -100}}}, wantErr: order.ErrInvalidOrder},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			repo := memory.NewOrderRepository()
			svc := service.NewOrderService(repo)

			got, err := svc.Create(context.Background(), tt.input)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.NotZero(t, got.ID)
			require.Equal(t, tt.wantTotal, got.TotalPrice)
			require.Equal(t, order.StatusNew, got.Status)
			require.NotZero(t, got.CreatedAt)
			require.Len(t, got.History, 1)
			require.Equal(t, order.StatusNew, got.History[0].Status)
		})
	}
}

func TestGetByID(t *testing.T) {
	repo := memory.NewOrderRepository()
	svc := service.NewOrderService(repo)

	created, _ := svc.Create(context.Background(), order.CreateOrderInput{Items: makeItems()})

	cases := []struct {
		name    string
		id      int64
		wantErr error
	}{
		{"success", created.ID, nil},
		{"not found", 999, order.ErrOrderNotFound},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.GetByID(context.Background(), tt.id)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, created.ID, got.ID)
		})
	}
}

func TestList(t *testing.T) {
	repo := memory.NewOrderRepository()
	svc := service.NewOrderService(repo)

	_, _ = svc.Create(context.Background(), order.CreateOrderInput{Items: makeItems()})
	_, _ = svc.Create(context.Background(), order.CreateOrderInput{Items: makeItems()})

	list, err := svc.List(context.Background())
	require.NoError(t, err)
	require.Len(t, list, 2)
}

func TestHistory(t *testing.T) {
	repo := memory.NewOrderRepository()
	svc := service.NewOrderService(repo)

	created, _ := svc.Create(context.Background(), order.CreateOrderInput{Items: makeItems()})
	_, _ = svc.UpdateStatus(context.Background(), created.ID, order.StatusProcessing)

	history, err := svc.History(context.Background(), created.ID)
	require.NoError(t, err)
	require.Len(t, history, 2)
	require.Equal(t, order.StatusNew, history[0].Status)
	require.Equal(t, order.StatusProcessing, history[1].Status)

	_, err = svc.History(context.Background(), 999)
	require.ErrorIs(t, err, order.ErrOrderNotFound)
}

func TestUpdateStatus(t *testing.T) {
	svc := service.NewOrderService(memory.NewOrderRepository())

	cases := []struct {
		name         string
		initialFlow  []order.Status
		targetStatus order.Status
		wantErr      error
	}{
		{"new to processing", nil, order.StatusProcessing, nil},
		{"new to cancelled", nil, order.StatusCancelled, nil},
		{"processing to completed", []order.Status{order.StatusProcessing}, order.StatusCompleted, nil},
		{"processing to cancelled", []order.Status{order.StatusProcessing}, order.StatusCancelled, nil},
		{"invalid new to completed", nil, order.StatusCompleted, order.ErrInvalidStatusTransition},
		{"invalid completed to processing", []order.Status{order.StatusProcessing, order.StatusCompleted}, order.StatusProcessing, order.ErrInvalidStatusTransition},
		{"invalid cancelled to new", []order.Status{order.StatusCancelled}, order.StatusNew, order.ErrInvalidStatusTransition},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			created, _ := svc.Create(context.Background(), order.CreateOrderInput{Items: makeItems()})

			for _, s := range tt.initialFlow {
				_, _ = svc.UpdateStatus(context.Background(), created.ID, s)
			}

			got, err := svc.UpdateStatus(context.Background(), created.ID, tt.targetStatus)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.targetStatus, got.Status)
			require.Len(t, got.History, len(tt.initialFlow)+2)

			for i := 1; i < len(got.History); i++ {
				require.False(t, got.History[i].ChangedAt.Before(got.History[i-1].ChangedAt))
			}
		})
	}
}
