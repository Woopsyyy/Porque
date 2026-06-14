// Package apperr defines typed application errors and their HTTP status mapping,
// so handlers can translate domain failures into responses uniformly.
package apperr

import (
	"errors"
	"fmt"
	"net/http"
)

// Kind classifies an error for HTTP status mapping.
type Kind int

const (
	KindInternal Kind = iota
	KindNotFound
	KindConflict
	KindValidation
	KindUnauthorized
	KindConflictState // operation invalid for the current state
)

// Error is a domain error carrying a Kind and a human-readable message.
type Error struct {
	Kind    Kind
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.Err }

// Constructors.
func NotFound(msg string) *Error     { return &Error{Kind: KindNotFound, Message: msg} }
func Conflict(msg string) *Error     { return &Error{Kind: KindConflict, Message: msg} }
func Validation(msg string) *Error   { return &Error{Kind: KindValidation, Message: msg} }
func Unauthorized(msg string) *Error { return &Error{Kind: KindUnauthorized, Message: msg} }
func BadState(msg string) *Error     { return &Error{Kind: KindConflictState, Message: msg} }
func Internal(err error) *Error {
	return &Error{Kind: KindInternal, Message: "internal error", Err: err}
}
func Wrap(k Kind, msg string, e error) *Error { return &Error{Kind: k, Message: msg, Err: e} }

// HTTPStatus returns the HTTP status code for an error, defaulting to 500.
func HTTPStatus(err error) int {
	var e *Error
	if !errors.As(err, &e) {
		return http.StatusInternalServerError
	}
	switch e.Kind {
	case KindNotFound:
		return http.StatusNotFound
	case KindConflict:
		return http.StatusConflict
	case KindConflictState:
		return http.StatusConflict
	case KindValidation:
		return http.StatusBadRequest
	case KindUnauthorized:
		return http.StatusUnauthorized
	default:
		return http.StatusInternalServerError
	}
}
