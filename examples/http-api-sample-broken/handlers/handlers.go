package handlers

import (
	"encoding/json"
	"net/http"
)

// GetOrderHandler returns an HTTP handler that retrieves an order by id.
func GetOrderHandler(s *Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		order, ok := s.Get(id)
		if !ok {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(order)
	})
}

// CreateOrderHandler returns an HTTP handler that creates a new order.
// BUG: missing validation branch — invalid input falls through to 201.
func CreateOrderHandler(s *Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body OrderInput
		_ = json.NewDecoder(r.Body).Decode(&body)
		// BUG: missing validation branch — invalid input falls through to 201.
		order := s.Create(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(order)
	})
}
