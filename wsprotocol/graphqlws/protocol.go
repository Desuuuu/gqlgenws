// Package graphqlws implements the graphql-ws protocol.
package graphqlws

import (
	"context"
	"net/http"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/Desuuuu/gqlgenws/internal/util"
	"github.com/Desuuuu/gqlgenws/wsutil"
	"nhooyr.io/websocket"
)

const ProtocolName = "graphql-transport-ws"

const defaultInitTimeout = 3 * time.Second

// Protocol implements the graphql-ws protocol described here:
// https://github.com/enisdenjo/graphql-ws.
//
// Protocol can be used as a gqlgenws.Protocol or directly as a gqlgen transport.
type Protocol struct {
	// InitFunc is called after receiving the "connection_init" message with the
	// WebSocket handshake HTTP request and the message payload.
	//
	// The returned Context, if not nil, is provided to GraphQL resolvers. When
	// the Context is done, the connection is also closed.
	//
	// The returned ObjectPayload, if not nil, is used as the payload for the
	// "connection_ack" message.
	//
	// If a non-nil error is returned, the connection is closed.
	//
	// If InitFunc is nil, all connections are accepted.
	InitFunc func(*http.Request, ObjectPayload) (context.Context, ObjectPayload, error)

	// InitTimeout is the duration to wait for a "connection_init" message before
	// closing the connection.
	//
	// Defaults to 3 seconds.
	InitTimeout time.Duration

	// If PingInterval is set, a "ping" message is sent if no message is
	// received for the specified duration.
	PingInterval time.Duration

	// AcceptOptions defines options used during the WebSocket handshake.
	AcceptOptions websocket.AcceptOptions
}

var _ graphql.Transport = &Protocol{}

func (*Protocol) Supports(r *http.Request) (res bool) {
	if !wsutil.IsUpgrade(r) {
		return false
	}

	if !util.HasHeader(r.Header, "Sec-WebSocket-Protocol") {
		return true
	}

	return util.HeaderContains(r.Header, "Sec-WebSocket-Protocol", ProtocolName)
}

func (p *Protocol) Do(w http.ResponseWriter, r *http.Request, exec graphql.GraphExecutor) {
	if len(p.AcceptOptions.Subprotocols) == 0 {
		p.AcceptOptions.Subprotocols = []string{ProtocolName}
	}

	c, err := websocket.Accept(w, r, &p.AcceptOptions)
	if err != nil {
		return
	}

	p.Run(r, c, exec)
}

func (*Protocol) Name() string {
	return ProtocolName
}

func (p *Protocol) Run(r *http.Request, c *websocket.Conn, exec graphql.GraphExecutor) {
	if p.InitTimeout.Nanoseconds() <= 0 {
		p.InitTimeout = defaultInitTimeout
	}

	conn := &connection{
		protocol: p,
		conn:     c,
		req:      r,
		ctx:      r.Context(),
		exec:     exec,
	}

	conn.close(conn.run())
}
