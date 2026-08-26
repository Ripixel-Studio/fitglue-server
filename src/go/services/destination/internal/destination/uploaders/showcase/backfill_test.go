package showcase

import (
	"reflect"
	"testing"
)

func TestUnionStrings(t *testing.T) {
	got := unionStrings([]string{"a", "b", ""}, []string{"b", "c", "a"})
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("unionStrings = %v, want %v", got, want)
	}
	if unionStrings(nil, nil) != nil {
		t.Error("union of nothing should be nil")
	}
}
