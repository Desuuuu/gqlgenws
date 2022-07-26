// Package wserr declares WebSocket error types and implements functions to pass
// WebSocket errors using context.
package wserr

import (
	"fmt"

	"nhooyr.io/websocket"
)

// CloseError represents a WebSocket close error.
type CloseError struct {
	// Code is sent to the client in the close frame.
	Code int

	// Reason is sent to the client in the close frame.
	Reason string

	Err error
}

func (e CloseError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%d: %s: %s", e.Code, e.Reason, e.Err.Error())
	}

	return fmt.Sprintf("%d: %s", e.Code, e.Reason)
}

func (e CloseError) Unwrap() error {
	return e.Err
}

func (e CloseError) StatusCode() websocket.StatusCode {
	return websocket.StatusCode(e.Code)
}
