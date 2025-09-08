package context

import (
	"context"
	"os"
	"testing"

	"github.com/krateoplatformops/unstructured-runtime/pkg/logging"
)

type mockLogger struct {
	test string
}

func (m mockLogger) Debug(msg string, args ...any)    {}
func (m mockLogger) Info(msg string, args ...any)     {}
func (m mockLogger) Warn(msg string, args ...any)     {}
func (m mockLogger) Error(error, string, ...any)      {}
func (m mockLogger) WithName(string) logging.Logger   { return m }
func (m mockLogger) WithValues(...any) logging.Logger { return m }

func TestLogger(t *testing.T) {
	t.Run("returns logger from context", func(t *testing.T) {
		mock := mockLogger{
			test: "value",
		}
		ctx := context.WithValue(context.Background(), contextKeyLogger, mock)

		result, ok := Logger(ctx).(mockLogger)
		if !ok {
			t.Error("expected logger of type mockLogger")
		}

		if result.test != "value" {
			t.Errorf("expected logger with test value 'value', got '%s'", result.test)
		}

	})

	t.Run("returns default logger when not in context", func(t *testing.T) {
		ctx := context.Background()

		result := Logger(ctx)

		if result == nil {
			t.Error("expected default logger")
		}
	})
}

func TestWithLogger(t *testing.T) {
	t.Run("adds provided logger to context", func(t *testing.T) {
		mock := mockLogger{}
		ctx := context.Background()

		withLoggerFunc := WithLogger(mock)
		newCtx := withLoggerFunc(ctx)

		result := newCtx.Value(contextKeyLogger)
		if result != mock {
			t.Error("expected logger to be added to context")
		}
	})

	t.Run("creates default logger when nil provided", func(t *testing.T) {
		ctx := context.Background()

		withLoggerFunc := WithLogger(nil)
		newCtx := withLoggerFunc(ctx)

		result := newCtx.Value(contextKeyLogger)
		if result == nil {
			t.Error("expected default logger to be created")
		}
	})

	t.Run("creates debug logger when DEBUG env var is true", func(t *testing.T) {
		os.Setenv("DEBUG", "true")
		defer os.Unsetenv("DEBUG")

		ctx := context.Background()

		withLoggerFunc := WithLogger(nil)
		newCtx := withLoggerFunc(ctx)

		result := newCtx.Value(contextKeyLogger)
		if result == nil {
			t.Error("expected debug logger to be created")
		}
	})

	t.Run("creates info logger when DEBUG env var is not true", func(t *testing.T) {
		os.Setenv("DEBUG", "false")
		defer os.Unsetenv("DEBUG")

		ctx := context.Background()

		withLoggerFunc := WithLogger(nil)
		newCtx := withLoggerFunc(ctx)

		result := newCtx.Value(contextKeyLogger)
		if result == nil {
			t.Error("expected info logger to be created")
		}
	})
}
func TestBuildContext(t *testing.T) {
	t.Run("returns original context when no options provided", func(t *testing.T) {
		ctx := context.Background()

		result := BuildContext(ctx)

		if result != ctx {
			t.Error("expected original context to be returned")
		}
	})

	t.Run("applies single WithContextFunc", func(t *testing.T) {
		ctx := context.Background()
		mock := mockLogger{test: "single"}

		result := BuildContext(ctx, WithLogger(mock))

		logger := result.Value(contextKeyLogger)
		if logger != mock {
			t.Error("expected logger to be added to context")
		}
	})

	t.Run("applies multiple WithContextFunc in order", func(t *testing.T) {
		ctx := context.Background()
		mock1 := mockLogger{test: "first"}
		mock2 := mockLogger{test: "second"}

		result := BuildContext(ctx, WithLogger(mock1), WithLogger(mock2))

		// The second logger should overwrite the first
		logger, ok := result.Value(contextKeyLogger).(mockLogger)
		if !ok {
			t.Error("expected logger of type mockLogger")
		}
		if logger.test != "second" {
			t.Errorf("expected logger with test value 'second', got '%s'", logger.test)
		}
	})

	t.Run("handles nil WithContextFunc gracefully", func(t *testing.T) {
		ctx := context.Background()
		mock := mockLogger{test: "test"}

		// This test ensures the function doesn't panic with mixed nil and valid functions
		var nilFunc WithContextFunc
		result := BuildContext(ctx, WithLogger(mock), nilFunc)

		// Should still have the logger from the valid function
		logger := result.Value(contextKeyLogger)
		if logger != mock {
			t.Error("expected logger to be added to context despite nil function")
		}
	})
}
