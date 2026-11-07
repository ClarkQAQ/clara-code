// Package errs defines stable error categories shared by Clara Core packages.
package errs

import (
	"errors"
	"fmt"
)

// Code identifies a stable error category.
type Code string

const (
	CodeInvalidArgument Code = "invalid_argument"
	CodeNotFound        Code = "not_found"
	CodeConflict        Code = "conflict"
	CodeBusy            Code = "busy"
	CodeCancelled       Code = "cancelled"
	CodeUnauthorized    Code = "unauthorized"
	CodeForbidden       Code = "forbidden"
	CodeUnsupported     Code = "unsupported"
	CodeLimit           Code = "limit"
	CodeInternal        Code = "internal"
)

// Error carries a stable code and an optional wrapped cause.
type Error struct {
	Code    Code
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Message == "" {
		return string(e.Code)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.Cause }

// New returns an error with a stable code.
func New(code Code, message string) error {
	return &Error{Code: code, Message: message}
}

// Wrap adds a stable code and context to err.
func Wrap(code Code, message string, err error) error {
	if err == nil {
		return nil
	}
	return &Error{Code: code, Message: message, Cause: err}
}

// CodeOf reports the stable code carried by err.
func CodeOf(err error) Code {
	var typed *Error
	if errors.As(err, &typed) {
		return typed.Code
	}
	return CodeInternal
}

// IsCode reports whether err carries code.
func IsCode(err error, code Code) bool {
	return err != nil && CodeOf(err) == code
}

var (
	ErrNotFound     = &Error{Code: CodeNotFound, Message: "not found"}
	ErrConflict     = &Error{Code: CodeConflict, Message: "conflict"}
	ErrBusy         = &Error{Code: CodeBusy, Message: "busy"}
	ErrCancelled    = &Error{Code: CodeCancelled, Message: "cancelled"}
	ErrUnauthorized = &Error{Code: CodeUnauthorized, Message: "unauthorized"}
	ErrForbidden    = &Error{Code: CodeForbidden, Message: "forbidden"}
	ErrUnsupported  = &Error{Code: CodeUnsupported, Message: "unsupported"}
)
