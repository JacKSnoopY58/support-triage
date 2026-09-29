package businesslogic

import (
	"reflect"
	"testing"

	"support-triage-service/model"
)

func TestFilterFAQIDsRejectsUnknownIDs(t *testing.T) {
	t.Parallel()
	got := FilterFAQIDs(
		[]string{"known", "invented"},
		[]model.KBHit{{ID: "known"}},
	)
	if want := []string{"known"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("FilterFAQIDs() = %v, want %v", got, want)
	}
}
