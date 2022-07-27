# gqlgenws

[![Go Doc Badge]][Go Doc] [![Go Report Card Badge]][Go Report Card]

WebSocket transports for [gqlgen](https://github.com/99designs/gqlgen).

Provided protocol implementations follow their respective specification as
closely as possible.

## Protocols

* [graphql-ws](https://github.com/enisdenjo/graphql-ws) in the `graphqlws` package.
* [subscriptions-transport-ws](https://github.com/apollographql/subscriptions-transport-ws) in the `transportws` package.

## Usage

```go
srv := handler.New(executableSchema)

initFunc := func(r *http.Request, p wsutil.ObjectPayload) (context.Context, wsutil.ObjectPayload, error) {
    ctx := r.Context()
    // ...
    return ctx, nil, nil
}

p1 := &graphqlws.Protocol{
    InitFunc: initFunc,
    PingInterval: 25 * time.Second,
}

p2 := &transportws.Protocol{
    InitFunc: initFunc,
    KeepAliveInterval: 25 * time.Second,
}

// Use the protocol directly as a transport.
srv.AddTransport(p1)
srv.AddTransport(p2)

// OR

// Use with Negotiator to do protocol negotiation.
srv.AddTransport(wstransport.NewNegotiator(p1, p2))
```

[Go Doc]: https://pkg.go.dev/github.com/Desuuuu/gqlgenws
[Go Doc Badge]: https://pkg.go.dev/badge/github.com/Desuuuu/gqlgenws.svg
[Go Report Card]: https://goreportcard.com/report/github.com/Desuuuu/gqlgenws
[Go Report Card Badge]: https://goreportcard.com/badge/github.com/Desuuuu/gqlgenws
