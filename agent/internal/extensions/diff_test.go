package extensions

import (
	"reflect"
	"testing"
)

func TestDiffClassifiesAndOrdersExtensionChanges(t *testing.T) {
	got, err := Diff([]string{"timescaledb", "pgvector", "pgvector"}, []string{"postgis", "powa"})
	if err != nil {
		t.Fatal(err)
	}
	want := []Action{{Type: ActionCreate, Extension: "pgvector", Method: UpdateMethodHotApply}, {Type: ActionCreate, Extension: "timescaledb", Method: UpdateMethodRestart}, {Type: ActionDrop, Extension: "postgis", Method: UpdateMethodHotApply}, {Type: ActionDrop, Extension: "powa", Method: UpdateMethodRestart}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("actions = %v, want %v", got, want)
	}
}
func TestDiffExtensionNoopAndUnsupportedNames(t *testing.T) {
	got, err := Diff([]string{"pgvector", "postgis"}, []string{"postgis", "pgvector"})
	if err != nil || len(got) != 0 {
		t.Fatalf("unchanged = %v, %v", got, err)
	}
	for _, tt := range []struct{ desired, actual []string }{{[]string{"unknown"}, nil}, {nil, []string{"unknown"}}} {
		if _, err := Diff(tt.desired, tt.actual); err == nil {
			t.Fatal("unsupported extension accepted")
		}
	}
}
