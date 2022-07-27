// Package wstransport implements common gqlgen WebSocket transports.
package wstransport

import (
	"net/http"

	"github.com/99designs/gqlgen/graphql"
	"github.com/Desuuuu/gqlgenws"
	"github.com/Desuuuu/gqlgenws/internal/util"
	"github.com/Desuuuu/gqlgenws/wsutil"
	"nhooyr.io/websocket"
)

// Negotiator is a gqlgen transport that accepts WebSocket connections.
//
// Negotiator negotiates a protocol with the client based on its registered
// protocols then delegates connection handling to the protocol.
//
// Negotiator returns an error to the client if no acceptable transport is
// found.
type Negotiator struct {
	// Default is used when a client does not request any specific protocol.
	//
	// Default can be nil.
	Default gqlgenws.Protocol

	// Protocols contains all supported protocols using their name as the key.
	Protocols map[string]gqlgenws.Protocol

	// AcceptOptions defines options used during the WebSocket handshake.
	AcceptOptions websocket.AcceptOptions
}

var _ graphql.Transport = &Negotiator{}

// NewNegotiator creates a Negotiator with the provided default protocol and
// any extra protocol supplied.
func NewNegotiator(def gqlgenws.Protocol, protocols ...gqlgenws.Protocol) *Negotiator {
	n := &Negotiator{
		Protocols: make(map[string]gqlgenws.Protocol, len(protocols)),
	}

	if def != nil {
		n.Default = def
		n.Protocols[def.Name()] = def
		n.AcceptOptions.Subprotocols = append(n.AcceptOptions.Subprotocols, def.Name())
	}

	for _, p := range protocols {
		n.Protocols[p.Name()] = p
		n.AcceptOptions.Subprotocols = append(n.AcceptOptions.Subprotocols, p.Name())
	}

	return n
}

func (n *Negotiator) Supports(r *http.Request) (res bool) {
	return wsutil.IsUpgrade(r)
}

func (n *Negotiator) Do(w http.ResponseWriter, r *http.Request, exec graphql.GraphExecutor) {
	if len(n.AcceptOptions.Subprotocols) < 1 {
		for name := range n.Protocols {
			n.AcceptOptions.Subprotocols = append(n.AcceptOptions.Subprotocols, name)
		}
	}

	c, err := websocket.Accept(w, r, &n.AcceptOptions)
	if err != nil {
		return
	}

	var protocol gqlgenws.Protocol

	s := c.Subprotocol()
	switch s {
	case "":
		if !util.HasHeader(r.Header, "Sec-WebSocket-Protocol") {
			protocol = n.Default
		}
	default:
		protocol = n.Protocols[s]
	}

	if protocol == nil {
		c.Close(websocket.StatusProtocolError, "subprotocol negotiation failed")
		return
	}

	protocol.Run(r, c, exec)
}
