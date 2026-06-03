package main

import (
	"log"
	"net/http"

	"example.com/http-api-sample-broken/handlers"
)

func main() {
	s := handlers.NewStore()
	s.Create(handlers.OrderInput{Item: "widget"}) // seed id=1

	mux := http.NewServeMux()
	mux.Handle("GET /orders/{id}", handlers.GetOrderHandler(s))
	mux.Handle("POST /orders", handlers.CreateOrderHandler(s))

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
