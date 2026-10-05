package api

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"syscall"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// ErrorKind classifies why a run failed, so a caller — an orchestrator reading
// the CLI's exit code, or Go code inspecting an error — can decide whether
// retrying could help. Only ErrUnavailable is worth retrying.
type ErrorKind int

const (
	// ErrConfig: the config is invalid (syntax, unknown key or type, wiring,
	// deadlock, run mode). Fix the config; retrying cannot help.
	ErrConfig ErrorKind = iota + 1
	// ErrUsage: the command line was wrong.
	ErrUsage
	// ErrData: an input was reachable but its contents could not be used.
	ErrData
	// ErrUnavailable: an input or output resource could not be reached — a
	// missing input file, a refused connection. Retrying later may succeed.
	ErrUnavailable
	// ErrRuntime: the simulation itself failed while running.
	ErrRuntime
)

// Exit codes, following BSD sysexits.h so an orchestrator can map them without
// stochadex-specific knowledge. A panic the engine cannot intercept (one raised
// inside a partition's worker goroutine) still exits with Go's own status 2,
// which callers should treat like ExitRuntime.
const (
	ExitOK          = 0
	ExitUsage       = 64 // EX_USAGE
	ExitData        = 65 // EX_DATAERR
	ExitRuntime     = 70 // EX_SOFTWARE
	ExitUnavailable = 75 // EX_TEMPFAIL: the one class worth retrying
	ExitConfig      = 78 // EX_CONFIG
)

// Error is a failure with its ErrorKind. It wraps the underlying error, so
// errors.Is / errors.As see through it.
type Error struct {
	Kind ErrorKind
	Err  error
}

func (e *Error) Error() string { return e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

// KindOf reports the ErrorKind of err: the kind it was classified with, or
// ErrRuntime for an unclassified failure. It returns 0 for a nil error.
func KindOf(err error) ErrorKind {
	if err == nil {
		return 0
	}
	var classified *Error
	if errors.As(err, &classified) {
		return classified.Kind
	}
	return ErrRuntime
}

// ExitCode maps err to the CLI exit code for its kind (ExitOK for nil).
func ExitCode(err error) int {
	switch KindOf(err) {
	case 0:
		return ExitOK
	case ErrUsage:
		return ExitUsage
	case ErrData:
		return ExitData
	case ErrUnavailable:
		return ExitUnavailable
	case ErrConfig:
		return ExitConfig
	default:
		return ExitRuntime
	}
}

// withKind classifies err as kind, unless it is nil or already classified.
func withKind(kind ErrorKind, err error) error {
	if err == nil {
		return nil
	}
	var classified *Error
	if errors.As(err, &classified) {
		return err
	}
	return &Error{Kind: kind, Err: err}
}

func configError(err error) error { return withKind(ErrConfig, err) }

// inputError classifies a failure reading an input: one that could not be
// reached — missing, not readable, or a network failure — is worth retrying;
// anything else means the input was there but unusable.
func inputError(err error) error {
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission) ||
		isNetworkError(err) {
		return withKind(ErrUnavailable, err)
	}
	return withKind(ErrData, err)
}

// isNetworkError reports a genuine network failure. It deliberately does not use
// the net.Error interface: syscall.Errno satisfies it, so every system error —
// disk full, is-a-directory — would count as a network failure.
func isNetworkError(err error) bool {
	var opErr *net.OpError
	var dnsErr *net.DNSError
	return errors.As(err, &opErr) || errors.As(err, &dnsErr) ||
		errors.Is(err, syscall.ECONNREFUSED)
}

// panicError classifies a value recovered from a panic raised while running: a
// sink's ResourceError (its destination could not be reached when the run
// started) or a network failure is ErrUnavailable; an already-classified error
// keeps its kind; anything else is ErrRuntime.
func panicError(recovered any) error {
	err, ok := recovered.(error)
	if !ok {
		err = fmt.Errorf("%v", recovered)
	}
	var resourceErr *simulator.ResourceError
	if errors.As(err, &resourceErr) || isNetworkError(err) {
		return withKind(ErrUnavailable, err)
	}
	return withKind(ErrRuntime, fmt.Errorf("simulation failed: %w", err))
}
