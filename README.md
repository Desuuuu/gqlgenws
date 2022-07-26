[![Go Report Card](https://goreportcard.com/badge/github.com/Desuuuu/gqlgenws)](https://goreportcard.com/report/github.com/Desuuuu/gqlgenws) [![Go Doc](https://pkg.go.dev/badge/github.com/Desuuuu/gqlgenws.svg)](https://pkg.go.dev/github.com/Desuuuu/gqlgenws)

# gqlgenws

WebSocket transport for [gqlgen](https://github.com/99designs/gqlgen) with
support for pluggable protocols.

## Protocols

For now, the only protocol provided is the
[graphql-ws](https://github.com/enisdenjo/graphql-ws) protocol. The
implementation tries to follow the
[spec](https://github.com/enisdenjo/graphql-ws/blob/master/PROTOCOL.md) as
closely as possible.

## Usage

```go
srv := handler.New(executableSchema)

p := &graphqlws.Protocol{
    InitFunc: func(r *http.Request, p ObjectPayload) (context.Context, ObjectPayload, error) {
        ctx := r.Context()
        // ...
        return ctx, nil, nil
    },
    PingInterval: 25 * time.Second,
}

// Use the protocol directly as a transport.
srv.AddTransport(p)

// OR

// Use with Negotiator to support multiple protocols.
srv.AddTransport(wstransport.NewNegotiator(p))
```
