package tester

import (
	"reflect"
	"testing"
)

// Eq asserts that got == want using reflect.DeepEqual for non-comparable types.
func Eq[T any](t *testing.T, got, want T, msgAndArgs ...any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		if len(msgAndArgs) > 0 {
			t.Fatalf("%v: got=%v want=%v", msgAndArgs[0], got, want)
		}
		t.Fatalf("got=%v want=%v", got, want)
	}
}

// True asserts that cond is true.
func True(t *testing.T, cond bool, msgAndArgs ...any) {
	t.Helper()
	if !cond {
		if len(msgAndArgs) > 0 {
			t.Fatalf("%v", msgAndArgs[0])
		}
		t.Fatalf("expected condition to be true")
	}
}

// False asserts that cond is false.
func False(t *testing.T, cond bool, msgAndArgs ...any) {
	t.Helper()
	if cond {
		if len(msgAndArgs) > 0 {
			t.Fatalf("%v", msgAndArgs[0])
		}
		t.Fatalf("expected condition to be false")
	}
}

// NoErr asserts that err is nil.
func NoErr(t *testing.T, err error, msgAndArgs ...any) {
	t.Helper()
	if err != nil {
		if len(msgAndArgs) > 0 {
			t.Fatalf("%v: %v", msgAndArgs[0], err)
		}
		t.Fatalf("unexpected error: %v", err)
	}
}
