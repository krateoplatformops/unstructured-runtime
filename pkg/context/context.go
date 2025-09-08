package context

import (
	"context"
	"log/slog"
	"os"

	"github.com/krateoplatformops/unstructured-runtime/pkg/logging"
)

type contextKey string
type WithContextFunc func(context.Context) context.Context

var (
	contextKeyLogger = contextKey("logger")
)

func Logger(ctx context.Context) logging.Logger {
	log, ok := ctx.Value(contextKeyLogger).(logging.Logger)
	if !ok {
		log = logging.NewSlogLogger(*slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})))
	}

	return log
}

func WithLogger(root logging.Logger) WithContextFunc {
	return func(ctx context.Context) context.Context {
		if root == nil {
			logLevel := slog.LevelInfo
			if os.Getenv("DEBUG") == "true" {
				logLevel = slog.LevelDebug
			}
			root = logging.NewSlogLogger(*slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
				Level: logLevel,
			})))
		}

		return context.WithValue(ctx, contextKeyLogger, root)
	}
}

func BuildContext(ctx context.Context, opts ...WithContextFunc) context.Context {
	for _, fn := range opts {
		if fn == nil {
			continue
		}
		ctx = fn(ctx)
	}

	return ctx
}
