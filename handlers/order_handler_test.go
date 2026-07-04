package handlers

// The stub below implements the sqlc-generated db.Querier interface
// (emit_interface: true in sqlc.yaml), letting handlers be tested
// without a live database.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"order-api/db"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// stubQueries implements Querier with canned responses.
type stubQueries struct {
	getOrdersFn   func(ctx context.Context, userID string) ([]db.Order, error)
	getOrderFn    func(ctx context.Context, arg db.GetOrderParams) (db.Order, error)
	createOrderFn func(ctx context.Context, arg db.CreateOrderParams) (db.Order, error)
	updateOrderFn func(ctx context.Context, arg db.UpdateOrderParams) (db.Order, error)
	deleteOrderFn func(ctx context.Context, arg db.DeleteOrderParams) (int64, error)
}

func (s *stubQueries) GetOrders(ctx context.Context, userID string) ([]db.Order, error) {
	return s.getOrdersFn(ctx, userID)
}
func (s *stubQueries) GetOrder(ctx context.Context, arg db.GetOrderParams) (db.Order, error) {
	return s.getOrderFn(ctx, arg)
}
func (s *stubQueries) CreateOrder(ctx context.Context, arg db.CreateOrderParams) (db.Order, error) {
	return s.createOrderFn(ctx, arg)
}
func (s *stubQueries) UpdateOrder(ctx context.Context, arg db.UpdateOrderParams) (db.Order, error) {
	return s.updateOrderFn(ctx, arg)
}
func (s *stubQueries) DeleteOrder(ctx context.Context, arg db.DeleteOrderParams) (int64, error) {
	return s.deleteOrderFn(ctx, arg)
}

func newTestMux(s *Server) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /orders", s.GetOrders)
	mux.HandleFunc("GET /orders/{id}", s.GetOrderByID)
	mux.HandleFunc("POST /orders", s.CreateOrder)
	mux.HandleFunc("PUT /orders/{id}", s.UpdateOrder)
	mux.HandleFunc("DELETE /orders/{id}", s.DeleteOrder)
	return mux
}

func TestGetOrders_OK(t *testing.T) {
	srv := &Server{Queries: &stubQueries{
		getOrdersFn: func(ctx context.Context, userID string) ([]db.Order, error) {
			// Requests without auth context resolve to the shared
			// anonymous owner — the query must still be owner-scoped.
			if userID != anonymousOwner {
				t.Errorf("userID = %q, want %q", userID, anonymousOwner)
			}
			return []db.Order{{ID: "ord_1", Item: "Keyboard", Quantity: 1, UserID: userID}}, nil
		},
	}}

	req := httptest.NewRequest(http.MethodGet, "/orders", nil)
	rec := httptest.NewRecorder()
	newTestMux(srv).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var got []db.Order
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if len(got) != 1 || got[0].ID != "ord_1" {
		t.Errorf("unexpected body: %+v", got)
	}
}

func TestGetOrders_EmptyIsJSONArray(t *testing.T) {
	srv := &Server{Queries: &stubQueries{
		getOrdersFn: func(ctx context.Context, userID string) ([]db.Order, error) {
			return nil, nil // sqlc returns a nil slice for zero rows
		},
	}}

	req := httptest.NewRequest(http.MethodGet, "/orders", nil)
	rec := httptest.NewRecorder()
	newTestMux(srv).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := strings.TrimSpace(rec.Body.String()); body != "[]" {
		t.Errorf("body = %q, want %q", body, "[]")
	}
}

func TestGetOrderByID_NotFound(t *testing.T) {
	srv := &Server{Queries: &stubQueries{
		getOrderFn: func(ctx context.Context, arg db.GetOrderParams) (db.Order, error) {
			return db.Order{}, pgx.ErrNoRows
		},
	}}

	req := httptest.NewRequest(http.MethodGet, "/orders/missing", nil)
	rec := httptest.NewRecorder()
	newTestMux(srv).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestCreateOrder_ValidationErrors(t *testing.T) {
	srv := &Server{Queries: &stubQueries{}} // must never be called

	tests := []struct {
		name string
		body string
	}{
		{"invalid json", `{not json`},
		{"missing item", `{"quantity":1,"price":9.99}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()
			newTestMux(srv).ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestCreateOrder_ServerGeneratesID(t *testing.T) {
	var captured db.CreateOrderParams
	srv := &Server{Queries: &stubQueries{
		createOrderFn: func(ctx context.Context, arg db.CreateOrderParams) (db.Order, error) {
			captured = arg
			return db.Order(arg), nil
		},
	}}

	// A client-supplied "id" must be ignored, not honored.
	body := `{"id":"ord_client_chosen","item":"Keyboard","quantity":1,"price":9.99}`
	req := httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader(body))
	rec := httptest.NewRecorder()
	newTestMux(srv).ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if !strings.HasPrefix(captured.ID, "ord_") || captured.ID == "ord_client_chosen" {
		t.Errorf("ID = %q, want server-generated ord_* id", captured.ID)
	}
	if captured.UserID != anonymousOwner {
		t.Errorf("UserID = %q, want %q", captured.UserID, anonymousOwner)
	}

	var got db.Order
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if got.ID != captured.ID {
		t.Errorf("response ID = %q, want the generated %q", got.ID, captured.ID)
	}
}

func TestUpdateOrder_NotFound(t *testing.T) {
	srv := &Server{Queries: &stubQueries{
		updateOrderFn: func(ctx context.Context, arg db.UpdateOrderParams) (db.Order, error) {
			return db.Order{}, pgx.ErrNoRows
		},
	}}

	body := `{"item":"Keyboard","quantity":1,"price":9.99}`
	req := httptest.NewRequest(http.MethodPut, "/orders/missing", strings.NewReader(body))
	rec := httptest.NewRecorder()
	newTestMux(srv).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestDeleteOrder_NotFoundOnZeroRows(t *testing.T) {
	srv := &Server{Queries: &stubQueries{
		deleteOrderFn: func(ctx context.Context, arg db.DeleteOrderParams) (int64, error) {
			return 0, nil
		},
	}}

	req := httptest.NewRequest(http.MethodDelete, "/orders/missing", nil)
	rec := httptest.NewRecorder()
	newTestMux(srv).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestDeleteOrder_OK(t *testing.T) {
	srv := &Server{Queries: &stubQueries{
		deleteOrderFn: func(ctx context.Context, arg db.DeleteOrderParams) (int64, error) {
			if arg.UserID != anonymousOwner {
				t.Errorf("UserID = %q, want %q", arg.UserID, anonymousOwner)
			}
			return 1, nil
		},
	}}

	req := httptest.NewRequest(http.MethodDelete, "/orders/ord_1", nil)
	rec := httptest.NewRecorder()
	newTestMux(srv).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestHealthz(t *testing.T) {
	rec := httptest.NewRecorder()
	Healthz("abc1234")(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var got map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if got["status"] != "ok" || got["version"] != "abc1234" {
		t.Errorf("unexpected body: %+v", got)
	}
}
