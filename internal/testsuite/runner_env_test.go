package testsuite

import (
	"reflect"
	"testing"
)

func TestPreflightStepsExactSequence(t *testing.T) {
	want := [][]string{
		{"go", "test", "./...", "-count=1"},
	}

	got := preflightSteps()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("preflight steps mismatch\nwant=%v\ngot=%v", want, got)
	}
}
