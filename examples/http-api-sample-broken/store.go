package main

import (
	"fmt"
	"sync"
)

type OrderInput struct {
	Item string `json:"item"`
}

type Order struct {
	ID   string `json:"id"`
	Item string `json:"item"`
}

type Store struct {
	mu     sync.Mutex
	orders map[string]Order
	next   int
}

func NewStore() *Store { return &Store{orders: make(map[string]Order)} }

func (s *Store) Create(in OrderInput) Order {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	id := fmt.Sprintf("%d", s.next)
	o := Order{ID: id, Item: in.Item}
	s.orders[id] = o
	return o
}

func (s *Store) Get(id string) (Order, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.orders[id]
	return o, ok
}
