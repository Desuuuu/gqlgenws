// Package wsutil implements some WebSocket utility functions.
package wsutil

import (
	"net/http"

	"github.com/Desuuuu/gqlgenws/internal/util"
)

// IsUpgrade reports whether an HTTP request is a WebSocket upgrade request.
func IsUpgrade(r *http.Request) bool {
	if r.Method != "GET" {
		return false
	}

	if !util.HeaderContains(r.Header, "Connection", "upgrade") {
		return false
	}

	return util.HeaderContains(r.Header, "Upgrade", "websocket")
}
