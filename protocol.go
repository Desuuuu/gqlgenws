package gqlgenws

import (
	"net/http"

	"github.com/99designs/gqlgen/graphql"
	"nhooyr.io/websocket"
)

// Protocol is implemented by WebSocket protocols.
type Protocol interface {
	// Name returns the WebSocket protocol name used by the
	// Sec-WebSocket-Protocol header.
	Name() string

	// Run is called after the request has been upgraded and the protocol has
	// been negotiated with the client.
	Run(*http.Request, *websocket.Conn, graphql.GraphExecutor)
}
