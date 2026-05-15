package service

import (
	"errors"
	"fmt"
)

// ErrUnavailable indicates the SDK client is not currently usable.
var ErrUnavailable = errors.New("nitronode not reachable")

// ValidationError represents invalid client-supplied input after request decoding.
type ValidationError struct {
	message string
	code    string
}

func (e ValidationError) Error() string {
	return e.message
}

func (e ValidationError) Code() string {
	if e.code == "" {
		return "invalid_request"
	}
	return e.code
}

// NotFoundError represents a missing logical resource.
type NotFoundError struct {
	message string
	code    string
}

func (e NotFoundError) Error() string {
	return e.message
}

func (e NotFoundError) Code() string {
	if e.code == "" {
		return "not_found"
	}
	return e.code
}

// ConflictError represents a write that conflicts with current state.
type ConflictError struct {
	message string
	code    string
}

func (e ConflictError) Error() string {
	return e.message
}

func (e ConflictError) Code() string {
	if e.code == "" {
		return "conflict"
	}
	return e.code
}

// UpstreamError represents a Nitronode operation failure.
type UpstreamError struct {
	message string
	err     error
}

func (e UpstreamError) Error() string {
	if e.err != nil && e.message != "" {
		return fmt.Sprintf("%s: %v", e.message, e.err)
	}
	if e.err != nil {
		return e.err.Error()
	}
	return e.message
}

func (e UpstreamError) Unwrap() error {
	return e.err
}

func invalidf(format string, args ...any) error {
	return ValidationError{message: fmt.Sprintf(format, args...)}
}

func invalidCodef(code string, format string, args ...any) error {
	return ValidationError{message: fmt.Sprintf(format, args...), code: code}
}

func notFoundf(format string, args ...any) error {
	return NotFoundError{message: fmt.Sprintf(format, args...)}
}

func conflictf(format string, args ...any) error {
	return ConflictError{message: fmt.Sprintf(format, args...)}
}

func conflictCodef(code string, format string, args ...any) error {
	return ConflictError{message: fmt.Sprintf(format, args...), code: code}
}

func upstreamf(err error, format string, args ...any) error {
	return UpstreamError{message: fmt.Sprintf(format, args...), err: err}
}
