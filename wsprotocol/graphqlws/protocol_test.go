package graphqlws

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/testserver"
	"github.com/Desuuuu/gqlgenws/wserr"
	"github.com/Desuuuu/gqlgenws/wsprotocol/graphqlws/code"
	"github.com/Desuuuu/gqlgenws/wsutil"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"nhooyr.io/websocket"
)

type contextKey string

func TestProtocolAsTransport(t *testing.T) {
	protocol := &Protocol{}

	h := testserver.New()
	h.AddTransport(protocol)

	srv := httptest.NewServer(h)
	defer srv.Close()

	t.Run("handle websocket requests with default protocol", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
		defer cancel()

		c, err := wsConnect(ctx, srv.URL, "")
		require.NoError(t, err)
		defer c.Close(websocket.StatusNormalClosure, "")

		rctx := c.CloseRead(ctx)

		err = c.Ping(rctx)
		require.NoError(t, err)
	})

	t.Run("handle websocket requests with known protocol", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
		defer cancel()

		c, err := wsConnect(ctx, srv.URL, protocol.Name())
		require.NoError(t, err)
		defer c.Close(websocket.StatusNormalClosure, "")

		rctx := c.CloseRead(ctx)

		err = c.Ping(rctx)
		require.NoError(t, err)
	})

	t.Run("ignore websocket requests with unknown protocol", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
		defer cancel()

		_, err := wsConnect(ctx, srv.URL, "foo")

		var we wsError
		require.ErrorAs(t, err, &we)

		require.GreaterOrEqual(t, we.StatusCode, http.StatusBadRequest)
		require.Less(t, we.StatusCode, http.StatusInternalServerError)

		require.JSONEq(t, `{"errors":[{"message":"transport not supported"}],"data":null}`, string(we.Body))
	})

	t.Run("ignore non-websocket requests", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, "GET", srv.URL, nil)
		require.NoError(t, err)

		res, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer res.Body.Close()

		require.GreaterOrEqual(t, res.StatusCode, http.StatusBadRequest)
		require.Less(t, res.StatusCode, http.StatusInternalServerError)

		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.JSONEq(t, `{"errors":[{"message":"transport not supported"}],"data":null}`, string(body))
	})
}

func TestProtocol(t *testing.T) {
	h := testserver.New()
	h.AddTransport(&Protocol{})

	srv := httptest.NewServer(h)
	defer srv.Close()

	t.Run("invalid message", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
		defer cancel()

		c, err := wsConnect(ctx, srv.URL, "")
		require.NoError(t, err)
		defer c.Close(websocket.StatusNormalClosure, "")

		err = c.Write(ctx, websocket.MessageText, []byte("foo"))
		require.NoError(t, err)

		_, _, err = c.Read(ctx)

		var ce websocket.CloseError
		require.ErrorAs(t, err, &ce)
		require.Equal(t, code.BadRequest, int(ce.Code))
	})

	t.Run("init", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
		defer cancel()

		c, err := wsConnect(ctx, srv.URL, "")
		require.NoError(t, err)
		defer c.Close(websocket.StatusNormalClosure, "")

		err = c.Write(ctx, websocket.MessageText, []byte(`{"type":"connection_init"}`))
		require.NoError(t, err)

		_, res, err := c.Read(ctx)
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"connection_ack"}`, string(res))
	})

	t.Run("multiple inits", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
		defer cancel()

		c, err := wsConnect(ctx, srv.URL, "")
		require.NoError(t, err)
		defer c.Close(websocket.StatusNormalClosure, "")

		err = c.Write(ctx, websocket.MessageText, []byte(`{"type":"connection_init"}`))
		require.NoError(t, err)

		_, res, err := c.Read(ctx)
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"connection_ack"}`, string(res))

		err = c.Write(ctx, websocket.MessageText, []byte(`{"type":"connection_init"}`))
		require.NoError(t, err)

		_, _, err = c.Read(ctx)

		var ce websocket.CloseError
		require.ErrorAs(t, err, &ce)
		require.Equal(t, code.TooManyInitialisationRequests, int(ce.Code))
	})

	t.Run("ping", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
		defer cancel()

		c, err := wsConnect(ctx, srv.URL, "")
		require.NoError(t, err)
		defer c.Close(websocket.StatusNormalClosure, "")

		err = c.Write(ctx, websocket.MessageText, []byte(`{"type":"ping"}`))
		require.NoError(t, err)

		_, res, err := c.Read(ctx)
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"pong"}`, string(res))
	})

	t.Run("ping with payload", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
		defer cancel()

		c, err := wsConnect(ctx, srv.URL, "")
		require.NoError(t, err)
		defer c.Close(websocket.StatusNormalClosure, "")

		err = c.Write(ctx, websocket.MessageText, []byte(`{"type":"ping","payload":{"foo":"bar"}}`))
		require.NoError(t, err)

		_, res, err := c.Read(ctx)
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"pong","payload":{"foo":"bar"}}`, string(res))
	})

	t.Run("subscribe", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
		defer cancel()

		c, err := wsConnect(ctx, srv.URL, "")
		require.NoError(t, err)
		defer c.Close(websocket.StatusNormalClosure, "")

		err = c.Write(ctx, websocket.MessageText, []byte(`{"type":"connection_init"}`))
		require.NoError(t, err)

		_, res, err := c.Read(ctx)
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"connection_ack"}`, string(res))

		err = c.Write(ctx, websocket.MessageText, []byte(`{"type":"subscribe","id":"foo","payload":{"query":"subscription { name }"}}`))
		require.NoError(t, err)

		h.SendNextSubscriptionMessage()

		_, res, err = c.Read(ctx)
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"next","id":"foo","payload":{"data":{"name":"test"}}}`, string(res))
	})

	t.Run("subscribe single result", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
		defer cancel()

		c, err := wsConnect(ctx, srv.URL, "")
		require.NoError(t, err)
		defer c.Close(websocket.StatusNormalClosure, "")

		err = c.Write(ctx, websocket.MessageText, []byte(`{"type":"connection_init"}`))
		require.NoError(t, err)

		_, res, err := c.Read(ctx)
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"connection_ack"}`, string(res))

		err = c.Write(ctx, websocket.MessageText, []byte(`{"type":"subscribe","id":"foo","payload":{"query":"query { name }"}}`))
		require.NoError(t, err)

		_, res, err = c.Read(ctx)
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"next","id":"foo","payload":{"data":{"name":"test"}}}`, string(res))

		_, res, err = c.Read(ctx)
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"complete","id":"foo"}`, string(res))
	})

	t.Run("subscribe without init", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
		defer cancel()

		c, err := wsConnect(ctx, srv.URL, "")
		require.NoError(t, err)
		defer c.Close(websocket.StatusNormalClosure, "")

		err = c.Write(ctx, websocket.MessageText, []byte(`{"type":"subscribe","id":"foo","payload":{"query":"query { name }"}}`))
		require.NoError(t, err)

		_, _, err = c.Read(ctx)

		var ce websocket.CloseError
		require.ErrorAs(t, err, &ce)
		require.Equal(t, code.Unauthorized, int(ce.Code))
	})

	t.Run("subscribe id re-use", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
		defer cancel()

		c, err := wsConnect(ctx, srv.URL, "")
		require.NoError(t, err)
		defer c.Close(websocket.StatusNormalClosure, "")

		err = c.Write(ctx, websocket.MessageText, []byte(`{"type":"connection_init"}`))
		require.NoError(t, err)

		_, res, err := c.Read(ctx)
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"connection_ack"}`, string(res))

		err = c.Write(ctx, websocket.MessageText, []byte(`{"type":"subscribe","id":"foo","payload":{"query":"subscription { name }"}}`))
		require.NoError(t, err)

		err = c.Write(ctx, websocket.MessageText, []byte(`{"type":"subscribe","id":"foo","payload":{"query":"subscription { name }"}}`))
		require.NoError(t, err)

		_, _, err = c.Read(ctx)

		var ce websocket.CloseError
		require.ErrorAs(t, err, &ce)
		require.Equal(t, code.SubscriberAlreadyExists, int(ce.Code))
	})

	t.Run("operation error", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
		defer cancel()

		h := handler.New(&graphql.ExecutableSchemaMock{
			ExecFunc: func(ctx context.Context) graphql.ResponseHandler {
				return func(ctx context.Context) *graphql.Response {
					wserr.SetOperationError(ctx, errors.New("Custom operation error"))
					return nil
				}
			},
			SchemaFunc: func() *ast.Schema {
				return gqlparser.MustLoadSchema(&ast.Source{Input: `
				type Subscription {
					name: String!
				}
			`})
			},
		})

		h.AddTransport(&Protocol{})

		srv := httptest.NewServer(h)
		defer srv.Close()

		c, err := wsConnect(ctx, srv.URL, "")
		require.NoError(t, err)
		defer c.Close(websocket.StatusNormalClosure, "")

		err = c.Write(ctx, websocket.MessageText, []byte(`{"type":"connection_init"}`))
		require.NoError(t, err)

		_, res, err := c.Read(ctx)
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"connection_ack"}`, string(res))

		err = c.Write(ctx, websocket.MessageText, []byte(`{"type":"subscribe","id":"foo","payload":{"query":"subscription { name }"}}`))
		require.NoError(t, err)

		_, res, err = c.Read(ctx)
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"error","id":"foo","payload":[{"message":"Custom operation error"}]}`, string(res))
	})

	t.Run("operation close error", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
		defer cancel()

		closeCode := 3333
		closeReason := "Custom operation error"

		h := handler.New(&graphql.ExecutableSchemaMock{
			ExecFunc: func(ctx context.Context) graphql.ResponseHandler {
				return func(ctx context.Context) *graphql.Response {
					wserr.SetOperationError(ctx, wserr.CloseError{
						Code:   closeCode,
						Reason: closeReason,
					})
					return nil
				}
			},
			SchemaFunc: func() *ast.Schema {
				return gqlparser.MustLoadSchema(&ast.Source{Input: `
				type Subscription {
					name: String!
				}
			`})
			},
		})

		h.AddTransport(&Protocol{})

		srv := httptest.NewServer(h)
		defer srv.Close()

		c, err := wsConnect(ctx, srv.URL, "")
		require.NoError(t, err)
		defer c.Close(websocket.StatusNormalClosure, "")

		err = c.Write(ctx, websocket.MessageText, []byte(`{"type":"connection_init"}`))
		require.NoError(t, err)

		_, res, err := c.Read(ctx)
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"connection_ack"}`, string(res))

		err = c.Write(ctx, websocket.MessageText, []byte(`{"type":"subscribe","id":"foo","payload":{"query":"subscription { name }"}}`))
		require.NoError(t, err)

		_, _, err = c.Read(ctx)

		var ce websocket.CloseError
		require.ErrorAs(t, err, &ce)
		require.Equal(t, closeCode, int(ce.Code))
		require.Equal(t, closeReason, ce.Reason)
	})

	t.Run("complete", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
		defer cancel()

		c, err := wsConnect(ctx, srv.URL, "")
		require.NoError(t, err)
		defer c.Close(websocket.StatusNormalClosure, "")

		err = c.Write(ctx, websocket.MessageText, []byte(`{"type":"connection_init"}`))
		require.NoError(t, err)

		_, res, err := c.Read(ctx)
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"connection_ack"}`, string(res))

		err = c.Write(ctx, websocket.MessageText, []byte(`{"type":"subscribe","id":"foo","payload":{"query":"subscription { name }"}}`))
		require.NoError(t, err)

		h.SendNextSubscriptionMessage()

		_, res, err = c.Read(ctx)
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"next","id":"foo","payload":{"data":{"name":"test"}}}`, string(res))

		err = c.Write(ctx, websocket.MessageText, []byte(`{"type":"complete","id":"foo"}`))
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)
		h.SendNextSubscriptionMessage()

		ctx, cancel = context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()

		_, _, err = c.Read(ctx)
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("parse error", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
		defer cancel()

		c, err := wsConnect(ctx, srv.URL, "")
		require.NoError(t, err)
		defer c.Close(websocket.StatusNormalClosure, "")

		err = c.Write(ctx, websocket.MessageText, []byte(`{"type":"connection_init"}`))
		require.NoError(t, err)

		_, res, err := c.Read(ctx)
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"connection_ack"}`, string(res))

		err = c.Write(ctx, websocket.MessageText, []byte(`{"type":"subscribe","id":"foo","payload":{"query":"!"}}`))
		require.NoError(t, err)

		_, res, err = c.Read(ctx)
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"error","id":"foo","payload":[{"message":"Unexpected !","locations":[{"line":1,"column":1}],"extensions":{"code":"GRAPHQL_PARSE_FAILED"}}]}`, string(res))
	})
}

func TestProtocol_InitFunc(t *testing.T) {
	t.Run("accept connection if InitFunc is not provided", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
		defer cancel()

		h := testserver.New()
		h.AddTransport(&Protocol{})

		srv := httptest.NewServer(h)
		defer srv.Close()

		c, err := wsConnect(ctx, srv.URL, "")
		require.NoError(t, err)
		defer c.Close(websocket.StatusNormalClosure, "")

		err = c.Write(ctx, websocket.MessageText, []byte(`{"type":"connection_init"}`))
		require.NoError(t, err)

		_, res, err := c.Read(ctx)
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"connection_ack"}`, string(res))
	})

	t.Run("accept connection if InitFunc returns no error", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
		defer cancel()

		h := testserver.New()
		h.AddTransport(&Protocol{
			InitFunc: func(r *http.Request, op wsutil.ObjectPayload) (context.Context, wsutil.ObjectPayload, error) {
				return nil, nil, nil
			},
		})

		srv := httptest.NewServer(h)
		defer srv.Close()

		c, err := wsConnect(ctx, srv.URL, "")
		require.NoError(t, err)
		defer c.Close(websocket.StatusNormalClosure, "")

		err = c.Write(ctx, websocket.MessageText, []byte(`{"type":"connection_init"}`))
		require.NoError(t, err)

		_, res, err := c.Read(ctx)
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"connection_ack"}`, string(res))
	})

	t.Run("reject connection if InitFunc returns an error", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
		defer cancel()

		h := testserver.New()
		h.AddTransport(&Protocol{
			InitFunc: func(r *http.Request, op wsutil.ObjectPayload) (context.Context, wsutil.ObjectPayload, error) {
				return nil, nil, errors.New("connection refused")
			},
		})

		srv := httptest.NewServer(h)
		defer srv.Close()

		c, err := wsConnect(ctx, srv.URL, "")
		require.NoError(t, err)
		defer c.Close(websocket.StatusNormalClosure, "")

		err = c.Write(ctx, websocket.MessageText, []byte(`{"type":"connection_init"}`))
		require.NoError(t, err)

		_, _, err = c.Read(ctx)

		var ce websocket.CloseError
		require.ErrorAs(t, err, &ce)
		require.Equal(t, code.Forbidden, int(ce.Code))
	})

	t.Run("reject connection with a CloseError in InitFunc", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
		defer cancel()

		closeCode := 3333
		closeReason := "Custom init error"

		h := testserver.New()
		h.AddTransport(&Protocol{
			InitFunc: func(r *http.Request, op wsutil.ObjectPayload) (context.Context, wsutil.ObjectPayload, error) {
				return nil, nil, wserr.CloseError{
					Code:   closeCode,
					Reason: closeReason,
				}
			},
		})

		srv := httptest.NewServer(h)
		defer srv.Close()

		c, err := wsConnect(ctx, srv.URL, "")
		require.NoError(t, err)
		defer c.Close(websocket.StatusNormalClosure, "")

		err = c.Write(ctx, websocket.MessageText, []byte(`{"type":"connection_init"}`))
		require.NoError(t, err)

		_, _, err = c.Read(ctx)

		var ce websocket.CloseError
		require.ErrorAs(t, err, &ce)
		require.Equal(t, closeCode, int(ce.Code))
		require.Equal(t, closeReason, ce.Reason)
	})

	t.Run("augment resolver context with InitFunc", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
		defer cancel()

		ctxKey := contextKey("foo")
		ctxValue := "bar"

		h := handler.New(&graphql.ExecutableSchemaMock{
			ExecFunc: func(ctx context.Context) graphql.ResponseHandler {
				value, _ := ctx.Value(ctxKey).(string)
				return graphql.OneShot(&graphql.Response{Data: []byte(strconv.Quote(value))})
			},
			SchemaFunc: func() *ast.Schema {
				return gqlparser.MustLoadSchema(&ast.Source{Input: `
				schema {
					query: Query
				}
				type Query {
					getValue: String!
				}
			`})
			},
		})

		h.AddTransport(&Protocol{
			InitFunc: func(r *http.Request, op wsutil.ObjectPayload) (context.Context, wsutil.ObjectPayload, error) {
				return context.WithValue(r.Context(), ctxKey, ctxValue), nil, nil
			},
		})

		srv := httptest.NewServer(h)
		defer srv.Close()

		c, err := wsConnect(ctx, srv.URL, "")
		require.NoError(t, err)
		defer c.Close(websocket.StatusNormalClosure, "")

		err = c.Write(ctx, websocket.MessageText, []byte(`{"type":"connection_init"}`))
		require.NoError(t, err)

		_, res, err := c.Read(ctx)
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"connection_ack"}`, string(res))

		err = c.Write(ctx, websocket.MessageText, []byte(`{"type":"subscribe","id":"foo","payload":{"query":"query { getValue }"}}`))
		require.NoError(t, err)

		_, res, err = c.Read(ctx)
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"next","id":"foo","payload":{"data":"`+ctxValue+`"}}`, string(res))

		_, res, err = c.Read(ctx)
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"complete","id":"foo"}`, string(res))
	})

	t.Run("connection times out with the context returned by InitFunc", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
		defer cancel()

		closeCode := 3333
		closeReason := "Custom timeout error"

		var ecancel context.CancelFunc

		h := testserver.New()
		h.AddTransport(&Protocol{
			InitFunc: func(r *http.Request, op wsutil.ObjectPayload) (context.Context, wsutil.ObjectPayload, error) {
				ctx, cancel := context.WithCancel(r.Context())
				ecancel = cancel
				return wserr.SetTimeoutError(ctx, wserr.CloseError{
					Code:   closeCode,
					Reason: closeReason,
				}), nil, nil
			},
		})

		srv := httptest.NewServer(h)
		defer srv.Close()

		c, err := wsConnect(ctx, srv.URL, "")
		require.NoError(t, err)
		defer c.Close(websocket.StatusNormalClosure, "")

		err = c.Write(ctx, websocket.MessageText, []byte(`{"type":"connection_init"}`))
		require.NoError(t, err)

		_, res, err := c.Read(ctx)
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"connection_ack"}`, string(res))

		ecancel()
		time.Sleep(100 * time.Millisecond)

		_, _, err = c.Read(ctx)

		var ce websocket.CloseError
		require.ErrorAs(t, err, &ce)
		require.Equal(t, closeCode, int(ce.Code))
		require.Equal(t, closeReason, ce.Reason)
	})

	t.Run("ack with payload returned by InitFunc", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
		defer cancel()

		h := testserver.New()
		h.AddTransport(&Protocol{
			InitFunc: func(r *http.Request, op wsutil.ObjectPayload) (context.Context, wsutil.ObjectPayload, error) {
				return nil, wsutil.ObjectPayload{
					"foo": "bar",
				}, nil
			},
		})

		srv := httptest.NewServer(h)
		defer srv.Close()

		c, err := wsConnect(ctx, srv.URL, "")
		require.NoError(t, err)
		defer c.Close(websocket.StatusNormalClosure, "")

		err = c.Write(ctx, websocket.MessageText, []byte(`{"type":"connection_init"}`))
		require.NoError(t, err)

		_, res, err := c.Read(ctx)
		require.NoError(t, err)
		require.JSONEq(t, `{"type":"connection_ack","payload":{"foo":"bar"}}`, string(res))
	})
}

func TestProtocol_InitTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
	defer cancel()

	h := testserver.New()
	h.AddTransport(&Protocol{
		InitTimeout: 50 * time.Millisecond,
	})

	srv := httptest.NewServer(h)
	defer srv.Close()

	c, err := wsConnect(ctx, srv.URL, "")
	require.NoError(t, err)
	defer c.Close(websocket.StatusNormalClosure, "")

	_, _, err = c.Read(ctx)

	var ce websocket.CloseError
	require.ErrorAs(t, err, &ce)
	require.Equal(t, code.ConnectionInitialisationTimeout, int(ce.Code))
}

func TestProtocol_PingInterval(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
	defer cancel()

	h := testserver.New()
	h.AddTransport(&Protocol{
		PingInterval: 20 * time.Millisecond,
	})

	srv := httptest.NewServer(h)
	defer srv.Close()

	c, err := wsConnect(ctx, srv.URL, "")
	require.NoError(t, err)
	defer c.Close(websocket.StatusNormalClosure, "")

	_, res, err := c.Read(ctx)
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"ping"}`, string(res))

	_, res, err = c.Read(ctx)
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"ping"}`, string(res))
}

func wsConnect(ctx context.Context, targetUrl string, protocol string) (*websocket.Conn, error) {
	u, err := url.Parse(targetUrl)
	if err != nil {
		return nil, nil
	}

	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	}

	var protocols []string
	if protocol != "" {
		protocols = append(protocols, protocol)
	}

	c, res, err := websocket.Dial(ctx, u.String(), &websocket.DialOptions{
		Subprotocols: protocols,
	})
	if err != nil {
		wsErr := wsError{
			Err: err,
		}

		if res != nil {
			wsErr.StatusCode = res.StatusCode
			wsErr.Body, _ = io.ReadAll(res.Body)
		}

		return nil, wsErr
	}

	return c, nil
}

type wsError struct {
	Err        error
	StatusCode int
	Body       []byte
}

func (e wsError) Error() string {
	return e.Err.Error()
}

func (e wsError) Unwrap() error {
	return e.Err
}
