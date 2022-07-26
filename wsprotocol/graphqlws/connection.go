package graphqlws

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/Desuuuu/gqlgenws/wserr"
	"github.com/Desuuuu/gqlgenws/wsprotocol/graphqlws/code"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"nhooyr.io/websocket"
)

type connection struct {
	protocol *Protocol
	conn     *websocket.Conn
	req      *http.Request
	ctx      context.Context
	exec     graphql.GraphExecutor

	initReceived      bool
	initReceivedMutex sync.Mutex
	acknowledged      bool
	operations        map[string]context.CancelFunc
	operationsMutex   sync.RWMutex
}

func (c *connection) run() error {
	c.initReceivedMutex.Lock()
	c.initReceived = false
	c.initReceivedMutex.Unlock()
	c.acknowledged = false
	c.operations = make(map[string]context.CancelFunc)

	initCtx, initCancel := context.WithTimeout(c.req.Context(), c.protocol.InitTimeout)
	defer initCancel()

	go c.initTimeout(initCtx)

	var pingTicker *time.Ticker

	if c.protocol.PingInterval.Nanoseconds() > 0 {
		pingTicker = time.NewTicker(c.protocol.PingInterval)

		go c.ping(pingTicker)
	}

	for {
		msg, err := c.readMessage()
		if err != nil {
			return err
		}

		if pingTicker != nil {
			pingTicker.Reset(c.protocol.PingInterval)
		}

		switch msg.Type {
		case connectionInitType:
			err = c.init(msg.Payload)
			if err != nil {
				return err
			}

			initCancel()
			c.acknowledged = true
		case pingType:
			err := c.writeMessage(&message{
				Type: pongType,
			}, msg.Payload)
			if err != nil {
				return err
			}
		case pongType:
		case subscribeType:
			err = c.subscribe(msg.Id, msg.Payload)
			if err != nil {
				return err
			}
		case completeType:
			c.operationsMutex.Lock()
			cancel := c.operations[msg.Id]
			delete(c.operations, msg.Id)
			c.operationsMutex.Unlock()

			if cancel != nil {
				cancel()
			}
		default:
			return wserr.CloseError{
				Code:   code.BadRequest,
				Reason: "Invalid message",
			}
		}
	}
}

func (c *connection) init(payload []byte) error {
	c.initReceivedMutex.Lock()
	if c.initReceived {
		c.initReceivedMutex.Unlock()

		return wserr.CloseError{
			Code:   code.TooManyInitialisationRequests,
			Reason: "Too many initialisation requests",
		}
	}
	c.initReceived = true
	c.initReceivedMutex.Unlock()

	var ackPayload ObjectPayload

	initFunc := c.protocol.InitFunc
	if initFunc != nil {
		var initPayload ObjectPayload

		err := decodePayload(payload, &initPayload)
		if err != nil {
			return err
		}

		ctx, payload, err := initFunc(c.req, initPayload)
		if err != nil {
			var ce wserr.CloseError
			if errors.As(err, &ce) {
				return ce
			}

			return wserr.CloseError{
				Err:    err,
				Code:   code.Forbidden,
				Reason: "Forbidden",
			}
		}

		if ctx != nil && ctx != c.ctx {
			go c.authTimeout(ctx)

			c.ctx = ctx
		}

		ackPayload = payload
	}

	return c.writeMessage(&message{
		Type: connectionAckType,
	}, ackPayload)
}

func (c *connection) subscribe(id string, payload []byte) error {
	if !c.acknowledged {
		return wserr.CloseError{
			Code:   code.Unauthorized,
			Reason: "Unauthorized",
		}
	}

	if id == "" {
		return wserr.CloseError{
			Code:   code.BadRequest,
			Reason: "Invalid message",
		}
	}

	var params *graphql.RawParams

	ctx := graphql.StartOperationTrace(c.ctx)
	start := graphql.Now()

	if err := decodePayload(payload, &params, useNumber); err != nil {
		return err
	}

	if params == nil {
		return wserr.CloseError{
			Code:   code.BadRequest,
			Reason: "Invalid payload",
		}
	}

	params.ReadTime = graphql.TraceTiming{
		Start: start,
		End:   graphql.Now(),
	}

	c.operationsMutex.Lock()
	if _, ok := c.operations[id]; ok {
		c.operationsMutex.Unlock()

		return wserr.CloseError{
			Code:   code.SubscriberAlreadyExists,
			Reason: fmt.Sprintf("Subscriber for %s already exists", id),
		}
	}

	ctx, cancel := context.WithCancel(ctx)
	c.operations[id] = cancel
	c.operationsMutex.Unlock()

	rc, err := c.exec.CreateOperationContext(ctx, params)
	if err != nil {
		resp := c.exec.DispatchError(graphql.WithOperationContext(ctx, rc), err)
		c.operationError(id, resp.Errors)
		return nil
	}

	go c.executeOperation(ctx, rc, id)

	return nil
}

func (c *connection) executeOperation(ctx context.Context, rc *graphql.OperationContext, id string) {
	ctx = wserr.PrepareOperationContext(ctx)

	responses, ctx := c.exec.DispatchOperation(ctx, rc)

	err := wserr.GetOperationError(ctx)
	if err != nil {
		c.close(err)
		return
	}

	for {
		response := responses(ctx)
		if response == nil {
			break
		}

		c.operationResponse(id, response)
	}

	err = wserr.GetOperationError(ctx)
	if err != nil {
		c.close(err)
		return
	}

	c.operationComplete(id)
}

func (c *connection) operationResponse(id string, resp *graphql.Response) {
	c.operationsMutex.RLock()
	_, ok := c.operations[id]
	c.operationsMutex.RUnlock()

	if !ok {
		return
	}

	err := c.writeMessage(&message{
		Id:   id,
		Type: nextType,
	}, resp)
	if err != nil {
		c.close(err)
	}
}

func (c *connection) operationComplete(id string) {
	c.operationsMutex.Lock()
	cancel, ok := c.operations[id]
	delete(c.operations, id)
	c.operationsMutex.Unlock()

	if !ok {
		return
	}

	cancel()

	err := c.writeMessage(&message{
		Id:   id,
		Type: completeType,
	}, nil)
	if err != nil {
		c.close(err)
	}
}

func (c *connection) operationError(id string, errs gqlerror.List) {
	c.operationsMutex.Lock()
	cancel, ok := c.operations[id]
	delete(c.operations, id)
	c.operationsMutex.Unlock()

	if !ok {
		return
	}

	cancel()

	err := c.writeMessage(&message{
		Id:   id,
		Type: errorType,
	}, errs)
	if err != nil {
		c.close(err)
	}
}

func (c *connection) initTimeout(ctx context.Context) {
	<-ctx.Done()

	c.initReceivedMutex.Lock()
	defer c.initReceivedMutex.Unlock()

	if errors.Is(ctx.Err(), context.DeadlineExceeded) && !c.initReceived {
		c.close(wserr.CloseError{
			Code:   code.ConnectionInitialisationTimeout,
			Reason: "Connection initialisation timeout",
		})
	}
}

func (c *connection) authTimeout(ctx context.Context) {
	select {
	case <-ctx.Done():
		err := wserr.GetTimeoutError(ctx)

		var ce wserr.CloseError
		if !errors.As(err, &ce) {
			ce = wserr.CloseError{
				Code:   code.Unauthorized,
				Reason: "Authorization timed out",
			}
		}

		c.close(ce)
	case <-c.req.Context().Done():
	}
}

func (c *connection) ping(t *time.Ticker) {
	for {
		select {
		case <-c.req.Context().Done():
			return
		case <-t.C:
			err := c.writeMessage(&message{
				Type: pingType,
			}, nil)
			if err != nil {
				c.close(err)
			}
		}
	}
}

func (c *connection) close(err error) {
	if err == nil {
		c.conn.Close(websocket.StatusNormalClosure, "Normal Closure")
		return
	}

	var ce wserr.CloseError
	if !errors.As(err, &ce) {
		c.conn.Close(code.InternalServerError, "Error")
		return
	}

	c.conn.Close(ce.StatusCode(), ce.Reason)
}

func (c *connection) readMessage() (*message, error) {
	_, data, err := c.conn.Read(c.req.Context())
	if err != nil {
		return nil, err
	}

	msg, err := decodeMessage(data)
	if err != nil {
		return nil, err
	}

	return msg, nil
}

func (c *connection) writeMessage(msg *message, payload interface{}) error {
	var err error

	msg.Payload, err = encodePayload(payload)
	if err != nil {
		return err
	}

	data, err := encodeMessage(msg)
	if err != nil {
		return err
	}

	return c.conn.Write(c.req.Context(), websocket.MessageText, data)
}
