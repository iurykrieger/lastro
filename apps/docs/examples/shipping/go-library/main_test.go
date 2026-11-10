package main

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	lastro "github.com/iurykrieger/lastro/sdks/go"
)

// answers replays the handoff answers the CLI walkthrough uses, so the example runs offline.
func answers(_ context.Context, req lastro.AskRequest) (json.RawMessage, error) {
	return os.ReadFile("../answers/" + req.Step + ".json")
}

func TestProveShipping(t *testing.T) {
	proof, err := proveShipping(context.Background(), answers, shippingFee)
	if err != nil {
		t.Fatal(err)
	}
	if !proof.OK || !strings.Contains(proof.Receipt, "Proved: 3") {
		t.Fatalf("expected a proof, got:\n%s", proof.Receipt)
	}
	buggy := func(in lastro.Env) (lastro.Env, error) {
		if in["subtotal"].(float64) == 100 && in["member"] == false {
			return lastro.Env{"fee": 15}, nil
		}
		return shippingFee(in)
	}
	proof, err = proveShipping(context.Background(), answers, buggy)
	if err != nil {
		t.Fatal(err)
	}
	if proof.OK || len(proof.Failures) != 1 || proof.Failures[0].ID != "C1" {
		t.Fatalf("expected C1 to fail, got %+v", proof.Failures)
	}
}
