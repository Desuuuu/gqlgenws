package wstransport

import (
	"context"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler/testserver"
	"github.com/stretchr/testify/require"
	"nhooyr.io/websocket"
)

func TestNegotiator(t *testing.T) {
	protocol := &mockProtocol{}

	handler := testserver.New()
	handler.AddTransport(NewNegotiator(protocol))

	srv := httptest.NewServer(handler)
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

		require.Equal(t, protocol.Clients(), 1)

		err = protocol.Cleanup(ctx)
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

		require.Equal(t, protocol.Clients(), 1)

		err = protocol.Cleanup(ctx)
		require.NoError(t, err)
	})

	t.Run("close websocket requests with unknown protocol", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
		defer cancel()

		c, err := wsConnect(ctx, srv.URL, "foo")
		require.NoError(t, err)
		defer c.Close(websocket.StatusNormalClosure, "")

		rctx := c.CloseRead(ctx)

		err = c.Ping(rctx)

		var ce websocket.CloseError
		require.ErrorAs(t, err, &ce)
		require.Equal(t, "subprotocol negotiation failed", ce.Reason)

		require.Equal(t, protocol.Clients(), 0)

		err = protocol.Cleanup(ctx)
		require.NoError(t, err)
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

		require.Equal(t, protocol.Clients(), 0)

		err = protocol.Cleanup(ctx)
		require.NoError(t, err)
	})
}

func wsConnect(ctx context.Context, targetUrl string, protocol string) (*websocket.Conn, error) {
	u, err := url.Parse(targetUrl)
	if err != nil {
		return nil, err
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

type mockProtocol struct {
	cancelFns map[int]context.CancelFunc
	mutex     sync.Mutex
}

func (*mockProtocol) Name() string {
	return "example"
}

func (p *mockProtocol) Run(r *http.Request, c *websocket.Conn, _ graphql.GraphExecutor) {
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	p.mutex.Lock()
	if p.cancelFns == nil {
		p.cancelFns = make(map[int]context.CancelFunc)
	}
	id := rand.Int()
	p.cancelFns[id] = cancel
	p.mutex.Unlock()

	<-c.CloseRead(ctx).Done()

	p.mutex.Lock()
	delete(p.cancelFns, id)
	p.mutex.Unlock()
}

func (p *mockProtocol) Clients() int {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	return len(p.cancelFns)
}

func (p *mockProtocol) Cleanup(ctx context.Context) error {
	p.mutex.Lock()
	for _, cancel := range p.cancelFns {
		cancel()
	}
	p.mutex.Unlock()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if p.Clients() == 0 {
				return nil
			}
		}
	}
}
