package model

import "errors"

type ErrorKind int

const (
	KindInvalidArgument ErrorKind = iota + 1
	KindNotFound
	KindConflict
)

type Error struct {
	Kind    ErrorKind
	Code    string
	Title   string
	Message string
}

func (e *Error) Error() string {
	return e.Message
}

func AsError(err error) (*Error, bool) {
	var domainErr *Error
	ok := errors.As(err, &domainErr)

	return domainErr, ok
}

func InvalidArgument(message string) *Error {
	return &Error{Kind: KindInvalidArgument, Code: "invalid_request", Title: "Invalid request", Message: message}
}

var (
	ErrNotFound = &Error{
		Kind:    KindNotFound,
		Code:    "trip_not_found",
		Title:   "Trip not found",
		Message: "Trip was not found",
	}
	ErrCompleted = &Error{
		Kind:    KindConflict,
		Code:    "trip_completed",
		Title:   "Trip completed",
		Message: "Trip is already completed",
	}
	ErrDriverBusy = &Error{
		Kind:    KindConflict,
		Code:    "driver_busy",
		Title:   "Driver busy",
		Message: "Driver already has an active trip",
	}
	ErrIdempotencyConflict = &Error{
		Kind:    KindConflict,
		Code:    "idempotency_conflict",
		Title:   "Idempotency conflict",
		Message: "Idempotency-Key was already used with a different request body",
	}
)
