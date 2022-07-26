package wserr

import (
	"context"
)

type contextKey string

const (
	timeoutErrorKey   = contextKey("wserr.timeout")
	operationErrorKey = contextKey("wserr.operation")
)

// SetTimeoutError returns a new context with a timeout error.
//
// It can be retrieved later with GetTimeoutError.
func SetTimeoutError(ctx context.Context, err error) context.Context {
	return context.WithValue(ctx, timeoutErrorKey, err)
}

// GetTimeoutError retrieves the timeout error stored in the context, or nil if
// no error has been stored.
func GetTimeoutError(ctx context.Context) error {
	e, _ := ctx.Value(timeoutErrorKey).(error)
	return e
}

// PrepareOperationContext prepares a context for use with SetOperationError and
// GetOperationError.
func PrepareOperationContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, operationErrorKey, new(error))
}

// SetOperationError stores an operation error in the context.
//
// It can be retrieved later with GetOperationError.
//
// The context must have been prepared with PrepareOperationContext, otherwise
// SetOperationError does nothing.
func SetOperationError(ctx context.Context, err error) {
	e, _ := ctx.Value(operationErrorKey).(*error)
	if e != nil {
		*e = err
	}
}

// GetOperationError retrieves the operation error stored in the context, or
// nil if no error has been stored.
//
// The context must have been prepared with PrepareOperationContext, otherwise
// GetOperationError returns nil.
func GetOperationError(ctx context.Context) error {
	e, _ := ctx.Value(operationErrorKey).(*error)
	if e != nil {
		return *e
	}

	return nil
}
