package service

import "errors"

// ErrUnavailable indicates the SDK client is not currently usable.
var ErrUnavailable = errors.New("clearnode not reachable")
