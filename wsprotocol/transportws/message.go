package transportws

import (
	"bytes"
	"encoding/json"
)

type messageType string

const (
	connectionInitType      = messageType("connection_init")
	connectionAckType       = messageType("connection_ack")
	connectionErrorType     = messageType("connection_error")
	connectionTerminateType = messageType("connection_terminate")
	startType               = messageType("start")
	stopType                = messageType("stop")
	completeType            = messageType("complete")
	dataType                = messageType("data")
	errorType               = messageType("error")
	keepAliveType           = messageType("ka")
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
	return &msg, err
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

	return dec.Decode(dst)
}

func useNumber(dec *json.Decoder) {
	dec.UseNumber()
}
