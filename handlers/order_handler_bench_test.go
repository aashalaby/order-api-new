package handlers

// Benchmarks for the handler layer. These reuse the stubQueries fake from
// order_handler_test.go, so they measure ONLY the HTTP routing + JSON
// encode/decode + price-conversion overhead — no database, no network.
//
// Run locally:
//   go test -bench=. -benchmem -run=^$ ./handlers/
//
// Compare two runs (e.g. before/after a change) with benchstat:
//   go test -bench=. -benchmem -count=10 -run=^$ ./handlers/ > old.txt
//   ...make changes...
//   go test -bench=. -benchmem -count=10 -run=^$ ./handlers/ > new.txt
//   go run golang.org/x/perf/cmd/benchstat@latest old.txt new.txt

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"order-api/db"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// mustNumeric builds a pgtype.Numeric for fixture data, failing the
// benchmark on error so errcheck stays satisfied and fixtures stay valid.
func mustNumeric(tb testing.TB, s string) pgtype.Numeric {
	tb.Helper()
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		tb.Fatalf("building numeric fixture from %q: %v", s, err)
	}
	return n
}

// makeOrders builds n synthetic orders for list benchmarks.
func makeOrders(tb testing.TB, n int) []db.Order {
	tb.Helper()
	price := mustNumeric(tb, "19.99")
	orders := make([]db.Order, n)
	for i := range orders {
		orders[i] = db.Order{
			ID:       fmt.Sprintf("ord_%06d", i),
			Item:     "Mechanical Keyboard",
			Quantity: int32(i%10 + 1),
			Price:    price,
			UserID:   anonymousOwner,
		}
	}
	return orders
}

// benchMux wires all five routes exactly like main.go so the router's
// method+path matching cost is included in the measurement.
func benchMux(s *Server) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /orders", s.GetOrders)
	mux.HandleFunc("GET /orders/{id}", s.GetOrderByID)
	mux.HandleFunc("POST /orders", s.CreateOrder)
	mux.HandleFunc("PUT /orders/{id}", s.UpdateOrder)
	mux.HandleFunc("DELETE /orders/{id}", s.DeleteOrder)
	return mux
}

// BenchmarkGetOrders measures list serialization at several payload sizes.
// This is the endpoint most sensitive to the missing pagination (a known
// POC simplification), so the scaling behavior here is the interesting part.
func BenchmarkGetOrders(b *testing.B) {
	for _, size := range []int{1, 100, 1000} {
		b.Run(fmt.Sprintf("orders_%d", size), func(b *testing.B) {
			orders := makeOrders(b, size)
			srv := &Server{Queries: &stubQueries{
				getOrdersFn: func(ctx context.Context, userID string) ([]db.Order, error) {
					return orders, nil
				},
			}}
			mux := benchMux(srv)

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				req := httptest.NewRequest(http.MethodGet, "/orders", nil)
				rec := httptest.NewRecorder()
				mux.ServeHTTP(rec, req)
				if rec.Code != http.StatusOK {
					b.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
				}
			}
		})
	}
}

// BenchmarkGetOrderByID measures the single-item path: routing with a
// path parameter + one struct encode.
func BenchmarkGetOrderByID(b *testing.B) {
	order := db.Order{
		ID:       "ord_000001",
		Item:     "Mechanical Keyboard",
		Quantity: 2,
		Price:    mustNumeric(b, "149.50"),
	}
	srv := &Server{Queries: &stubQueries{
		getOrderFn: func(ctx context.Context, arg db.GetOrderParams) (db.Order, error) {
			return order, nil
		},
	}}
	mux := benchMux(srv)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/orders/ord_000001", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
	}
}

// BenchmarkCreateOrder measures the full write path minus the DB:
// JSON decode -> validation -> float64->Numeric conversion -> JSON encode.
func BenchmarkCreateOrder(b *testing.B) {
	srv := &Server{Queries: &stubQueries{
		createOrderFn: func(ctx context.Context, arg db.CreateOrderParams) (db.Order, error) {
			return db.Order(arg), nil
		},
	}}
	mux := benchMux(srv)
	// No client-supplied id anymore: the server generates order IDs, so
	// this bench now also includes the crypto/rand id generation cost.
	const payload = `{"item":"Mechanical Keyboard","quantity":2,"price":149.50}`

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			b.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
		}
	}
}

// BenchmarkUpdateOrder mirrors CreateOrder but exercises the PUT route.
func BenchmarkUpdateOrder(b *testing.B) {
	srv := &Server{Queries: &stubQueries{
		updateOrderFn: func(ctx context.Context, arg db.UpdateOrderParams) (db.Order, error) {
			return db.Order(arg), nil
		},
	}}
	mux := benchMux(srv)
	const payload = `{"item":"Mechanical Keyboard","quantity":3,"price":99.95}`

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPut, "/orders/ord_bench", strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
	}
}

// BenchmarkPriceConversion isolates the float64 -> pgtype.Numeric hop.
// "sprintf" is the approach the handlers use today; "strconv" is a
// lower-allocation candidate replacement, included so benchstat can
// quantify whether switching is worth it.
func BenchmarkPriceConversion(b *testing.B) {
	const price = 149.50

	b.Run("sprintf", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var n pgtype.Numeric
			if err := n.Scan(fmt.Sprintf("%.2f", price)); err != nil {
				b.Fatalf("scan: %v", err)
			}
		}
	})

	b.Run("strconv", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var n pgtype.Numeric
			if err := n.Scan(strconv.FormatFloat(price, 'f', 2, 64)); err != nil {
				b.Fatalf("scan: %v", err)
			}
		}
	})
}
