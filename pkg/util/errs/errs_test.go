package errs

import (
	"errors"
	"testing"
)

func TestWrapAndCode(t *testing.T) {
	cause := errors.New("cause")
	err := Wrap(CodeConflict, "already exists", cause)
	if CodeOf(err) != CodeConflict || !errors.Is(err, cause) {
		t.Fatalf("err = %v", err)
	}
	if !IsCode(err, CodeConflict) {
		t.Fatal("code did not match")
	}
}
