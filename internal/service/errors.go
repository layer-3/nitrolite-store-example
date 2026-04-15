package service

import (
	"errors"
	"fmt"
)

// ErrUnavailable indicates the SDK client is not currently usable.
var ErrUnavailable = errors.New("clearnode not reachable")

// ValidationError represents invalid client-supplied input after request decoding.
type ValidationError struct {
	message string
}

func (e ValidationError) Error() string {
	return e.message
}

// NotFoundError represents a missing logical resource.
type NotFoundError struct {
	message string
}

func (e NotFoundError) Error() string {
	return e.message
}

// ConflictError represents a write that conflicts with current state.
type ConflictError struct {
	message string
}

func (e ConflictError) Error() string {
	return e.message
}

func invalidf(format string, args ...any) error {
	return ValidationError{message: fmt.Sprintf(format, args...)}
}

func notFoundf(format string, args ...any) error {
	return NotFoundError{message: fmt.Sprintf(format, args...)}
}

func conflictf(format string, args ...any) error {
	return ConflictError{message: fmt.Sprintf(format, args...)}
}
