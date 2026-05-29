package handlers

import (
	"fmt"
	"sync"
)

// OrderInput is the request body for POST /orders.
type OrderInput struct {
	Item string `json:"item"`
}

// Order is the response body for order endpoints.
type Order struct {
	ID   string `json:"id"`
	Item string `json:"item"`
}

// Store is a simple in-memory order store.
type Store struct {
	mu     sync.Mutex
	orders map[string]Order
	next   int
}

// NewStore returns an empty Store.
func NewStore() *Store { return &Store{orders: make(map[string]Order)} }

// Create inserts a new order and returns it.
func (s *Store) Create(in OrderInput) Order {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	id := fmt.Sprintf("%d", s.next)
	o := Order{ID: id, Item: in.Item}
	s.orders[id] = o
	return o
}

// Get retrieves an order by id.
func (s *Store) Get(id string) (Order, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.orders[id]
	return o, ok
}
