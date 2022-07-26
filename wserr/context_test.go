package wserr

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetSetTimeoutError(t *testing.T) {
	ctx := context.TODO()
	err := errors.New("foo")

	require.Nil(t, GetTimeoutError(ctx))
	ctx = SetTimeoutError(ctx, err)
	require.Equal(t, err, GetTimeoutError(ctx))
}

func TestPrepareOperationContext(t *testing.T) {
	t.Run("cancellation propagation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.TODO())

		ctx = PrepareOperationContext(ctx)
		cancel()

		require.ErrorIs(t, ctx.Err(), context.Canceled)
	})

	t.Run("values propagation", func(t *testing.T) {
		ctxKey := contextKey("foo")
		ctxValue := "bar"

		ctx := context.WithValue(context.TODO(), ctxKey, ctxValue)
		ctx = PrepareOperationContext(ctx)

		require.Equal(t, ctx.Value(ctxKey), ctxValue)
	})
}

func TestGetSetOperationError(t *testing.T) {
	t.Run("prepared context", func(t *testing.T) {
		ctx := PrepareOperationContext(context.TODO())
		err := errors.New("foo")

		require.Nil(t, GetOperationError(ctx))
		SetOperationError(ctx, err)
		require.Equal(t, err, GetOperationError(ctx))
	})

	t.Run("unprepared context", func(t *testing.T) {
		ctx := context.TODO()
		err := errors.New("foo")

		require.Nil(t, GetOperationError(ctx))
		SetOperationError(ctx, err)
		require.Nil(t, GetOperationError(ctx))
	})
}
