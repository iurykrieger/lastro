package fixturebinder

import (
	"reflect"
	"testing"
)

func TestCollectFixtureRefs(t *testing.T) {
	got, err := CollectFixtureRefs(
		`curl ${{ fixtures.body }} ${{ inputs.method }}`,
		map[string]string{"extra": `${{ fixtures.headers }}`},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"body", "headers"} // sorted, deduped
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestCollectFixtureRefsNone(t *testing.T) {
	got, err := CollectFixtureRefs(`echo hi`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}
