package usecase

import (
	"strings"
	"testing"
)

func TestValidateIDAccepts(t *testing.T) {
	cases := []string{
		"a",
		"abc",
		"create-order",
		"x-1-2-3",
		"x" + strings.Repeat("a", 127), // exactly 128
	}
	for _, c := range cases {
		if err := ValidateID(c); err != nil {
			t.Errorf("ValidateID(%q) err: %v", c, err)
		}
	}
}

func TestValidateIDRejectsCharset(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "USECASE_ID_CHARSET"},
		{"-leading-hyphen", "USECASE_ID_CHARSET"},
		{"1-leading-digit", "USECASE_ID_CHARSET"},
		{"Has_Underscore", "USECASE_ID_CHARSET"},
		{"has.dot", "USECASE_ID_CHARSET"},
		{"UPPER", "USECASE_ID_CHARSET"},
		{"a b", "USECASE_ID_CHARSET"},
	}
	for _, c := range cases {
		err := ValidateID(c.in)
		if err == nil {
			t.Errorf("ValidateID(%q) ok; want %s", c.in, c.want)
			continue
		}
		ve, ok := err.(*ValidationError)
		if !ok {
			t.Errorf("ValidateID(%q): want *ValidationError, got %T", c.in, err)
			continue
		}
		if ve.Code != c.want {
			t.Errorf("ValidateID(%q) code = %q, want %q", c.in, ve.Code, c.want)
		}
	}
}

func TestValidateIDRejectsOverLength(t *testing.T) {
	tooLong := "a" + strings.Repeat("b", 128) // 129 chars
	err := ValidateID(tooLong)
	if err == nil {
		t.Fatal("want error for length 129")
	}
	ve, ok := err.(*ValidationError)
	if !ok || ve.Code != "USECASE_ID_TOO_LONG" {
		t.Errorf("got %v; want USECASE_ID_TOO_LONG", err)
	}
}
