package main

import "testing"

func TestClassify(t *testing.T) {
	if classify(1) != "positive" {
		t.Fail()
	}
}
