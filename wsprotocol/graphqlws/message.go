package graphqlws

import (
	"bytes"
	"encoding/json"

	"github.com/Desuuuu/gqlgenws/wserr"
	"github.com/Desuuuu/gqlgenws/wsprotocol/graphqlws/code"
)

type messageType string

const (
	connectionInitType = messageType("connection_init")
	connectionAckType  = messageType("connection_ack")
	pingType           = messageType("ping")
	pongType           = messageType("pong")
	subscribeType      = messageType("subscribe")
	nextType           = messageType("next")
	errorType          = messageType("error")
	completeType       = messageType("complete")
)

type message struct {
	Id      string          `json:"id,omitempty"`
	Type    messageType     `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

func encodeMessage(m *message) ([]byte, error) {
	return json.Marshal(m)
}

func decodeMessage(data []byte) (*message, error) {
	var msg message

	dec := json.NewDecoder(bytes.NewReader(data))

	err := dec.Decode(&msg)
	if err != nil {
		return nil, wserr.CloseError{
			Err:    err,
			Code:   code.BadRequest,
			Reason: "Invalid message",
		}
	}

	return &msg, nil
}

func encodePayload(payload interface{}) (json.RawMessage, error) {
	if payload == nil {
		return nil, nil
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	if string(data) == "null" {
		return nil, nil
	}

	return data, nil
}

func decodePayload(data []byte, dst interface{}, opts ...func(*json.Decoder)) error {
	if data == nil {
		return nil
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	for _, fn := range opts {
		fn(dec)
	}

	err := dec.Decode(dst)
	if err != nil {
		return wserr.CloseError{
			Err:    err,
			Code:   code.BadRequest,
			Reason: "Invalid payload",
		}
	}

	return nil
}

func useNumber(dec *json.Decoder) {
	dec.UseNumber()
}
