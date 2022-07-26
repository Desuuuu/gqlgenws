package wsutil

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsUpgrade(t *testing.T) {
	t.Run("valid upgrade", func(t *testing.T) {
		r, err := http.NewRequest("GET", "http://example.com", nil)
		require.NoError(t, err)

		r.Header.Set("Connection", "keep-alive, upgrade")
		r.Header.Set("Upgrade", "websocket, example")

		res := IsUpgrade(r)
		require.True(t, res)
	})

	t.Run("wrong method", func(t *testing.T) {
		r, err := http.NewRequest("POST", "http://example.com", nil)
		require.NoError(t, err)

		r.Header.Set("Connection", "upgrade")
		r.Header.Set("Upgrade", "websocket")

		res := IsUpgrade(r)
		require.False(t, res)
	})

	t.Run("no connection header", func(t *testing.T) {
		r, err := http.NewRequest("GET", "http://example.com", nil)
		require.NoError(t, err)

		r.Header.Set("Upgrade", "websocket")

		res := IsUpgrade(r)
		require.False(t, res)
	})

	t.Run("wrong connection header", func(t *testing.T) {
		r, err := http.NewRequest("GET", "http://example.com", nil)
		require.NoError(t, err)

		r.Header.Set("Connection", "keep-alive")
		r.Header.Set("Upgrade", "websocket")

		res := IsUpgrade(r)
		require.False(t, res)
	})

	t.Run("no upgrade header", func(t *testing.T) {
		r, err := http.NewRequest("GET", "http://example.com", nil)
		require.NoError(t, err)

		r.Header.Set("Connection", "upgrade")

		res := IsUpgrade(r)
		require.False(t, res)
	})

	t.Run("wrong upgrade header", func(t *testing.T) {
		r, err := http.NewRequest("GET", "http://example.com", nil)
		require.NoError(t, err)

		r.Header.Set("Connection", "upgrade")
		r.Header.Set("Upgrade", "example")

		res := IsUpgrade(r)
		require.False(t, res)
	})
}
