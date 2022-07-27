package transportws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/Desuuuu/gqlgenws/internal/util"
	"github.com/Desuuuu/gqlgenws/wserr"
	"github.com/Desuuuu/gqlgenws/wsutil"
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

	var keepAliveTicker *time.Ticker

	for {
		msg, err := c.readMessage()
		if err != nil {
			return err
		}

		if keepAliveTicker != nil {
			keepAliveTicker.Reset(c.protocol.KeepAliveInterval)
		}

		if msg == nil {
			continue
		}

		switch msg.Type {
		case connectionInitType:
			ok, err := c.init(msg.Payload)
			if err != nil {
				return err
			}

			if !ok {
				continue
			}

			initCancel()
			c.acknowledged = true

			if c.protocol.KeepAliveInterval.Nanoseconds() > 0 {
				err = c.writeMessage(&message{
					Type: keepAliveType,
				}, nil)
				if err != nil {
					return err
				}

				keepAliveTicker = time.NewTicker(c.protocol.KeepAliveInterval)

				go c.keepAlive(keepAliveTicker)
			}
		case startType:
			err = c.start(msg.Id, msg.Payload)
			if err != nil {
				return err
			}
		case stopType:
			c.stop(msg.Id)
		case connectionTerminateType:
			return nil
		default:
			err = c.writeMessage(&message{
				Type: errorType,
			}, wsutil.ObjectPayload{
				"message": "Invalid message type",
			})
			if err != nil {
				return err
			}
		}
	}
}

func (c *connection) init(payload json.RawMessage) (bool, error) {
	c.initReceivedMutex.Lock()
	if c.initReceived {
		c.initReceivedMutex.Unlock()

		return false, wserr.CloseError{
			Code:   int(websocket.StatusPolicyViolation),
			Reason: "Too many initialisation requests",
		}
	}
	c.initReceived = true
	c.initReceivedMutex.Unlock()

	var ackPayload wsutil.ObjectPayload

	initFunc := c.protocol.InitFunc
	if initFunc != nil {
		var initPayload wsutil.ObjectPayload

		err := decodePayload(payload, &initPayload)
		if err != nil {
			return false, c.writeMessage(&message{
				Type: connectionErrorType,
			}, wsutil.ObjectPayload{
				"message": "Invalid payload",
			})
		}

		ctx, payload, err := initFunc(c.req, initPayload)
		if err != nil {
			var ce wserr.CloseError
			if errors.As(err, &ce) {
				return false, ce
			}

			return false, wserr.CloseError{
				Err:    err,
				Code:   int(websocket.StatusPolicyViolation),
				Reason: "Forbidden",
			}
		}

		if ctx != nil && ctx != c.ctx {
			go c.authTimeout(ctx)

			c.ctx = ctx
		}

		ackPayload = payload
	}

	return true, c.writeMessage(&message{
		Type: connectionAckType,
	}, ackPayload)
}

func (c *connection) start(id string, payload json.RawMessage) error {
	if !c.acknowledged {
		return c.writeMessage(&message{
			Type: connectionErrorType,
		}, wsutil.ObjectPayload{
			"message": "Unauthorized",
		})
	}

	if id == "" {
		return c.writeMessage(&message{
			Type: connectionErrorType,
		}, wsutil.ObjectPayload{
			"message": "Invalid message",
		})
	}

	var params *graphql.RawParams

	ctx := graphql.StartOperationTrace(c.ctx)
	start := graphql.Now()

	if err := decodePayload(payload, &params, useNumber); err != nil || params == nil {
		return c.writeMessage(&message{
			Type: connectionErrorType,
		}, wsutil.ObjectPayload{
			"message": "Invalid payload",
		})
	}

	params.ReadTime = graphql.TraceTiming{
		Start: start,
		End:   graphql.Now(),
	}

	c.operationsMutex.Lock()
	if _, ok := c.operations[id]; ok {
		c.operationsMutex.Unlock()

		return c.writeMessage(&message{
			Type: connectionErrorType,
		}, wsutil.ObjectPayload{
			"message": fmt.Sprintf("Subscriber for %s already exists", id),
		})
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

func (c *connection) stop(id string) {
	c.operationsMutex.Lock()
	cancel := c.operations[id]
	delete(c.operations, id)
	c.operationsMutex.Unlock()

	if cancel != nil {
		cancel()
	}
}

func (c *connection) executeOperation(ctx context.Context, rc *graphql.OperationContext, id string) {
	ctx = wserr.PrepareOperationContext(ctx)

	responses, ctx := c.exec.DispatchOperation(ctx, rc)

	err := wserr.GetOperationError(ctx)
	if err == nil {
		for {
			response := responses(ctx)
			if response == nil {
				break
			}

			c.operationResponse(id, response)
		}

		err = wserr.GetOperationError(ctx)
	}

	if err != nil {
		var ce wserr.CloseError
		if errors.As(err, &ce) {
			c.close(ce)
			return
		}

		resp := c.exec.DispatchError(graphql.WithOperationContext(ctx, rc), util.GetErrorList(err))
		c.operationError(id, resp.Errors)
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
		Type: dataType,
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

	var opErr interface{}
	if len(errs) > 0 {
		opErr = errs[0]
	} else {
		opErr = wsutil.ObjectPayload{
			"message": "Error",
		}
	}

	err := c.writeMessage(&message{
		Id:   id,
		Type: errorType,
	}, opErr)
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
			Code:   int(websocket.StatusPolicyViolation),
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
				Code:   int(websocket.StatusPolicyViolation),
				Reason: "Authorization timed out",
			}
		}

		c.close(ce)
	case <-c.req.Context().Done():
	}
}

func (c *connection) keepAlive(t *time.Ticker) {
	for {
		select {
		case <-c.req.Context().Done():
			return
		case <-t.C:
			err := c.writeMessage(&message{
				Type: keepAliveType,
			}, nil)
			if err != nil {
				c.close(err)
			}
		}
	}
}

func (c *connection) close(err error) {
	if err == nil {
		c.conn.Close(websocket.StatusNormalClosure, "")
		return
	}

	var ce wserr.CloseError
	if !errors.As(err, &ce) {
		c.conn.Close(websocket.StatusInternalError, "Error")
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
		return nil, c.writeMessage(&message{
			Type: connectionErrorType,
		}, wsutil.ObjectPayload{
			"message": "Invalid message",
		})
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
