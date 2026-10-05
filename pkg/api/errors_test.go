package api

import (
	"errors"
	"io/fs"
	"syscall"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestErrorClassification(t *testing.T) {
	t.Run("each kind maps to its sysexits code", func(t *testing.T) {
		cases := map[ErrorKind]int{
			ErrUsage: 64, ErrData: 65, ErrRuntime: 70, ErrUnavailable: 75, ErrConfig: 78,
		}
		for kind, want := range cases {
			if got := ExitCode(&Error{Kind: kind, Err: errors.New("x")}); got != want {
				t.Errorf("kind %d: exit code %d, want %d", kind, got, want)
			}
		}
		if ExitCode(nil) != 0 {
			t.Error("a nil error should exit 0")
		}
		if ExitCode(errors.New("unclassified")) != ExitRuntime {
			t.Error("an unclassified error should be treated as a runtime failure")
		}
	})
	t.Run("classification keeps the cause reachable", func(t *testing.T) {
		err := inputError(&fs.PathError{Op: "open", Path: "x.csv", Err: fs.ErrNotExist})
		if KindOf(err) != ErrUnavailable || !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("a missing input should be unavailable and still match fs.ErrNotExist: %v", err)
		}
		resource := panicError(&simulator.ResourceError{Resource: "sink", Err: errors.New("denied")})
		var target *simulator.ResourceError
		if KindOf(resource) != ErrUnavailable || !errors.As(resource, &target) {
			t.Errorf("a sink ResourceError should be unavailable and still unwrap: %v", resource)
		}
	})
	t.Run("a system error during a run is not mistaken for a network failure", func(t *testing.T) {
		// syscall.Errno satisfies the net.Error interface; classifying by that
		// interface would make every errno "unavailable".
		err := panicError(&fs.PathError{Op: "write", Path: "x", Err: syscall.EISDIR})
		if KindOf(err) != ErrRuntime {
			t.Errorf("an errno outside a sink's ResourceError should be a runtime failure, got %d", KindOf(err))
		}
		if KindOf(inputError(&fs.PathError{Op: "read", Path: "x", Err: syscall.EISDIR})) != ErrData {
			t.Error("an unreadable-as-data input (EISDIR) should be a data error")
		}
	})
	t.Run("an error keeps its first classification", func(t *testing.T) {
		err := configError(withKind(ErrData, errors.New("bad row")))
		if KindOf(err) != ErrData {
			t.Errorf("re-classifying should not overwrite the original kind, got %d", KindOf(err))
		}
	})
}
