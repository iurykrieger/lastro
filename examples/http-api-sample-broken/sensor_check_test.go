package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/http-api-sample-broken/handlers"
)

// TestBadInputReturns400 is the e2e check exercised by the
// uc-create-order-bad-input-e2e-test sensor. It runs as part of the
// sensor's step (`go test -run TestBadInputReturns400`).
//
// In this sample the handler is broken — it returns 201 instead of 400 —
// so this test is expected to FAIL until /heal applies the EditPlan
// from heal-fixture/editplan.json.
func TestBadInputReturns400(t *testing.T) {
	s := handlers.NewStore()
	h := handlers.CreateOrderHandler(s)
	req := httptest.NewRequest("POST", "/orders", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		body, _ := io.ReadAll(w.Result().Body)
		t.Fatalf("want 400, got %d: %s", w.Code, body)
	}
	var env map[string]any
	if err := json.NewDecoder(bytes.NewReader(w.Body.Bytes())).Decode(&env); err != nil {
		t.Fatalf("non-JSON body: %v: %s", err, w.Body.String())
	}
	if _, ok := env["error"]; !ok {
		t.Fatalf("body missing error field: %s", w.Body.String())
	}
}
