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
func CreateOrderHandler(s *Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body OrderInput
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"error":"bad json"}`, http.StatusBadRequest)
			return
		}
		if body.Item == "" {
			http.Error(w, `{"error":"missing required field: item"}`, http.StatusBadRequest)
			return
		}
		order := s.Create(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(order)
	})
}
