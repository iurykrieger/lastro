package main

import (
	"log"
	"net/http"
)

func main() {
	s := NewStore()
	s.Create(OrderInput{Item: "widget"}) // seed id=1

	mux := http.NewServeMux()
	mux.Handle("GET /orders/{id}", GetOrderHandler(s))
	mux.Handle("POST /orders", CreateOrderHandler(s))

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
