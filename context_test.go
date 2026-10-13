// Copyright (c) 2026 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package zap

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/internal/exit"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

type ctxKey string

const correlationIDKey ctxKey = "correlation-id"

const correlationID = "e9718aab-aa1a-4ad8-b1e6-690f6c43bd15"

// correlationOption returns a Context option that promotes the correlation ID
// stored on the context (if any) to a structured field.
func correlationOption() Option {
	return Context(func(ctx context.Context) []Field {
		if id, ok := ctx.Value(correlationIDKey).(string); ok {
			return []Field{String(string(correlationIDKey), id)}
		}
		return nil
	})
}

func ctxWithCorrelationID() context.Context {
	return context.WithValue(context.Background(), correlationIDKey, correlationID)
}

// fieldMap collapses an observed entry's context into a key->value map for
// convenient assertions.
func fieldMap(fields []Field) map[string]interface{} {
	enc := zapcore.NewMapObjectEncoder()
	for _, f := range fields {
		f.AddTo(enc)
	}
	return enc.Fields
}

func TestLoggerContextFields(t *testing.T) {
	t.Run("context fields are prepended to log-site fields", func(t *testing.T) {
		core, logs := observer.New(zapcore.DebugLevel)
		logger := New(core, correlationOption())
		ctx := ctxWithCorrelationID()

		logger.InfoCtx(ctx, "hello", String("extra", "value"))

		entries := logs.All()
		require.Len(t, entries, 1)
		// Context fields come first, then log-site fields.
		require.Len(t, entries[0].Context, 2)
		assert.Equal(t, string(correlationIDKey), entries[0].Context[0].Key)
		assert.Equal(t, "extra", entries[0].Context[1].Key)

		got := fieldMap(entries[0].Context)
		assert.Equal(t, correlationID, got[string(correlationIDKey)])
		assert.Equal(t, "value", got["extra"])
	})

	t.Run("every leveled Ctx method emits context fields", func(t *testing.T) {
		core, logs := observer.New(zapcore.DebugLevel)
		logger := New(core, correlationOption())
		ctx := ctxWithCorrelationID()

		logger.DebugCtx(ctx, "m")
		logger.InfoCtx(ctx, "m")
		logger.WarnCtx(ctx, "m")
		logger.ErrorCtx(ctx, "m")
		logger.LogCtx(ctx, InfoLevel, "m")

		entries := logs.All()
		require.Len(t, entries, 5)
		for _, e := range entries {
			assert.Equal(t, correlationID, fieldMap(e.Context)[string(correlationIDKey)])
		}
	})

	// Regression test for the latent bug in the original PR #1019: when no
	// Context option is configured (or ctx is nil), the explicit log-site
	// fields must still be emitted instead of being dropped.
	t.Run("log-site fields preserved without Context option", func(t *testing.T) {
		core, logs := observer.New(zapcore.DebugLevel)
		logger := New(core) // no Context option
		ctx := ctxWithCorrelationID()

		logger.InfoCtx(ctx, "hello", String("extra", "value"))

		entries := logs.All()
		require.Len(t, entries, 1)
		require.Len(t, entries[0].Context, 1)
		assert.Equal(t, "value", fieldMap(entries[0].Context)["extra"])
	})

	t.Run("log-site fields preserved with nil context", func(t *testing.T) {
		core, logs := observer.New(zapcore.DebugLevel)
		logger := New(core, correlationOption())

		logger.InfoCtx(nil, "hello", String("extra", "value")) //nolint:staticcheck // intentionally nil

		entries := logs.All()
		require.Len(t, entries, 1)
		require.Len(t, entries[0].Context, 1)
		assert.Equal(t, "value", fieldMap(entries[0].Context)["extra"])
	})

	t.Run("DPanic/Panic/Fatal Ctx methods", func(t *testing.T) {
		core, logs := observer.New(zapcore.DebugLevel)
		logger := New(core, correlationOption())
		ctx := ctxWithCorrelationID()

		logger.DPanicCtx(ctx, "dpanic")
		assert.Panics(t, func() { logger.PanicCtx(ctx, "panic") })
		stub := exit.WithStub(func() { logger.FatalCtx(ctx, "fatal") })
		assert.True(t, stub.Exited, "Expected FatalCtx to exit the process.")

		for _, e := range logs.All() {
			assert.Equal(t, correlationID, fieldMap(e.Context)[string(correlationIDKey)])
		}
	})
}

func TestSugaredLoggerContextFields(t *testing.T) {
	ctx := ctxWithCorrelationID()

	t.Run("w-style methods prepend context fields", func(t *testing.T) {
		core, logs := observer.New(zapcore.DebugLevel)
		sugar := New(core, correlationOption()).Sugar()

		sugar.InfowCtx(ctx, "hello", "extra", "value")

		entries := logs.All()
		require.Len(t, entries, 1)
		got := fieldMap(entries[0].Context)
		assert.Equal(t, correlationID, got[string(correlationIDKey)])
		assert.Equal(t, "value", got["extra"])
	})

	t.Run("f-style methods attach context fields", func(t *testing.T) {
		core, logs := observer.New(zapcore.DebugLevel)
		sugar := New(core, correlationOption()).Sugar()

		sugar.InfofCtx(ctx, "hello %s", "world")

		entries := logs.All()
		require.Len(t, entries, 1)
		assert.Equal(t, "hello world", entries[0].Message)
		assert.Equal(t, correlationID, fieldMap(entries[0].Context)[string(correlationIDKey)])
	})

	t.Run("print-style and ln-style methods attach context fields", func(t *testing.T) {
		core, logs := observer.New(zapcore.DebugLevel)
		sugar := New(core, correlationOption()).Sugar()

		sugar.InfoCtx(ctx, "print style")
		sugar.InfolnCtx(ctx, "ln style")

		entries := logs.All()
		require.Len(t, entries, 2)
		for _, e := range entries {
			assert.Equal(t, correlationID, fieldMap(e.Context)[string(correlationIDKey)])
		}
	})

	t.Run("Log/Logw/Logf/Logln Ctx variants", func(t *testing.T) {
		core, logs := observer.New(zapcore.DebugLevel)
		sugar := New(core, correlationOption()).Sugar()

		sugar.LogCtx(ctx, InfoLevel, "m")
		sugar.LogwCtx(ctx, InfoLevel, "m", "extra", "value")
		sugar.LogfCtx(ctx, InfoLevel, "m %d", 1)
		sugar.LoglnCtx(ctx, InfoLevel, "m")

		entries := logs.All()
		require.Len(t, entries, 4)
		for _, e := range entries {
			assert.Equal(t, correlationID, fieldMap(e.Context)[string(correlationIDKey)])
		}
	})

	t.Run("w-style log-site pairs preserved without Context option", func(t *testing.T) {
		core, logs := observer.New(zapcore.DebugLevel)
		sugar := New(core).Sugar() // no Context option

		sugar.InfowCtx(ctx, "hello", "extra", "value")

		entries := logs.All()
		require.Len(t, entries, 1)
		require.Len(t, entries[0].Context, 1)
		assert.Equal(t, "value", fieldMap(entries[0].Context)["extra"])
	})

	t.Run("Panic/Fatal w-style Ctx methods", func(t *testing.T) {
		core, logs := observer.New(zapcore.DebugLevel)
		sugar := New(core, correlationOption()).Sugar()

		assert.Panics(t, func() { sugar.PanicwCtx(ctx, "panic", "extra", "value") })
		stub := exit.WithStub(func() { sugar.FatalwCtx(ctx, "fatal") })
		assert.True(t, stub.Exited, "Expected FatalwCtx to exit the process.")

		for _, e := range logs.All() {
			assert.Equal(t, correlationID, fieldMap(e.Context)[string(correlationIDKey)])
		}
	})
}

// ctxCore records the context seen by Check and Write.
type ctxCore struct {
	zapcore.Core

	checkCtxs []context.Context
}

func (c *ctxCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	c.checkCtxs = append(c.checkCtxs, ent.Context())
	if c.Enabled(ent.Level) {
		return ce.AddCore(ent, c)
	}
	return ce
}

func newCtxCore(lvl zapcore.Level) (*ctxCore, *observer.ObservedLogs) {
	core, logs := observer.New(lvl)
	return &ctxCore{Core: core}, logs
}

func TestContextAvailableToCoresAndHooks(t *testing.T) {
	ctx := ctxWithCorrelationID()

	t.Run("Logger Ctx methods", func(t *testing.T) {
		core, logs := newCtxCore(zapcore.DebugLevel)
		logger := New(core) // no Context option needed

		logger.DebugCtx(ctx, "m")
		logger.InfoCtx(ctx, "m")
		logger.WarnCtx(ctx, "m")
		logger.ErrorCtx(ctx, "m")
		logger.DPanicCtx(ctx, "m")
		logger.LogCtx(ctx, InfoLevel, "m")
		assert.Panics(t, func() { logger.PanicCtx(ctx, "m") })
		stub := exit.WithStub(func() { logger.FatalCtx(ctx, "m") })
		assert.True(t, stub.Exited)

		require.Len(t, core.checkCtxs, 8)
		for _, c := range core.checkCtxs {
			assert.Equal(t, ctx, c, "Core.Check should see the log-site context.")
		}
		require.Len(t, logs.All(), 8)
		for _, e := range logs.All() {
			assert.Equal(t, ctx, e.Entry.Context(), "Core.Write should see the log-site context.")
		}
	})

	t.Run("non-Ctx methods use Background", func(t *testing.T) {
		core, logs := newCtxCore(zapcore.DebugLevel)
		logger := New(core)

		logger.Info("m")
		logger.Sugar().Infow("m")

		for _, c := range core.checkCtxs {
			assert.Equal(t, context.Background(), c)
		}
		for _, e := range logs.All() {
			assert.Equal(t, context.Background(), e.Entry.Context())
		}
	})

	t.Run("CheckCtx", func(t *testing.T) {
		core, logs := newCtxCore(zapcore.DebugLevel)
		logger := New(core, correlationOption())

		ce := logger.CheckCtx(ctx, InfoLevel, "m")
		require.NotNil(t, ce)
		assert.Equal(t, ctx, ce.Context())
		ce.Write(String("extra", "value"))

		require.Equal(t, []context.Context{ctx}, core.checkCtxs)
		entries := logs.All()
		require.Len(t, entries, 1)
		assert.Equal(t, ctx, entries[0].Entry.Context())
		assert.Equal(t, map[string]interface{}{"extra": "value"}, fieldMap(entries[0].Context),
			"CheckCtx should not apply the Context option.")
	})

	t.Run("Hooks option", func(t *testing.T) {
		var seen []context.Context
		core, _ := observer.New(zapcore.DebugLevel)
		logger := New(core, Hooks(func(ent zapcore.Entry) error {
			seen = append(seen, ent.Context())
			return nil
		}))

		logger.InfoCtx(ctx, "m")
		logger.Sugar().InfowCtx(ctx, "m")
		logger.Info("m")

		assert.Equal(t, []context.Context{ctx, ctx, context.Background()}, seen)
	})

	t.Run("panic hook", func(t *testing.T) {
		core, _ := observer.New(zapcore.DebugLevel)
		hook := &ctxWriteHook{}
		logger := New(core, WithPanicHook(hook))

		logger.PanicCtx(ctx, "m")

		assert.Equal(t, ctx, hook.ctx)
	})

	t.Run("SugaredLogger Ctx methods", func(t *testing.T) {
		core, logs := newCtxCore(zapcore.DebugLevel)
		sugar := New(core).Sugar()

		sugar.InfoCtx(ctx, "m")
		sugar.InfofCtx(ctx, "m %d", 1)
		sugar.InfowCtx(ctx, "m", "k", "v")
		sugar.InfolnCtx(ctx, "m")
		sugar.LogCtx(ctx, InfoLevel, "m")
		sugar.LogfCtx(ctx, InfoLevel, "m %d", 1)
		sugar.LogwCtx(ctx, InfoLevel, "m", "k", "v")
		sugar.LoglnCtx(ctx, InfoLevel, "m")

		require.Len(t, core.checkCtxs, 8)
		for _, c := range core.checkCtxs {
			assert.Equal(t, ctx, c)
		}
		for _, e := range logs.All() {
			assert.Equal(t, ctx, e.Entry.Context())
		}
	})

	t.Run("Context option not invoked for disabled levels", func(t *testing.T) {
		core, logs := observer.New(zapcore.InfoLevel)
		var calls int
		logger := New(core, Context(func(context.Context) []Field {
			calls++
			return nil
		}))
		sugar := logger.Sugar()

		logger.DebugCtx(ctx, "m")
		sugar.DebugCtx(ctx, "m")
		sugar.DebugfCtx(ctx, "m")
		sugar.DebugwCtx(ctx, "m")
		sugar.DebuglnCtx(ctx, "m")
		assert.Zero(t, calls)
		assert.Zero(t, logs.Len())
	})
}

type ctxWriteHook struct {
	ctx context.Context
}

func (h *ctxWriteHook) OnWrite(ce *zapcore.CheckedEntry, _ []zapcore.Field) {
	h.ctx = ce.Context()
}
