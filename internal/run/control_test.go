package run

import (
	"errors"
	"testing"
)

func TestControlErrorKeepsMessage(t *testing.T) {
	inner := errors.New("CreateJobObjectW: synthetic")
	err := error(controlError{inner})
	if err.Error() != inner.Error() || !errors.Is(err, ErrProcessControl) || !errors.Is(err, inner) {
		t.Fatalf("control error %v", err)
	}
	if errors.Is(inner, ErrProcessControl) {
		t.Fatal("a plain start error is marked as process control")
	}
}
