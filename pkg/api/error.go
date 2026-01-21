package api

import "errors"

var ErrNotFound = errors.New("not found")

// ErrNamespaceMismatch indicates a resource exists but in a different namespace than requested.
var ErrNamespaceMismatch = errors.New("namespace mismatch")
