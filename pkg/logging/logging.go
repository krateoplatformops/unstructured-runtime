package logging

import (
	"github.com/go-logr/logr"
)

// A Logger logs messages. Messages may be supplemented by structured data.
type Logger interface {
	// Info logs a message with optional structured data. Structured data must
	// be supplied as an array that alternates between string keys and values of
	// an arbitrary type. Use Info for messages that operators are
	// very likely to be concerned with when running.
	Info(msg string, keysAndValues ...any)

	// Debug logs a message with optional structured data. Structured data must
	// be supplied as an array that alternates between string keys and values of
	// an arbitrary type. Use Debug for messages that operators or
	// developers may be concerned with when debugging.
	Debug(msg string, keysAndValues ...any)

	// Error logs an error with a message and optional structured data.
	// Structured data must be supplied as an array that alternates between
	// string keys and values of an arbitrary type. Use Error for messages that
	// operators or developers should be concerned with when debugging.
	Error(err error, msg string, keysAndValues ...any)

	// WithValues returns a Logger that will include the supplied structured
	// data with any subsequent messages it logs. Structured data must
	// be supplied as an array that alternates between string keys and values of
	// an arbitrary type.
	WithValues(keysAndValues ...any) Logger

	// WithName returns a Logger that will include the supplied name with any
	// subsequent messages it logs. The name is typically a component name or
	// similar, and should be used to distinguish between different components
	// or subsystems.
	WithName(name string) Logger
}

// NewNopLogger returns a Logger that does nothing.
func NewNopLogger() Logger { return nopLogger{} }

type nopLogger struct{}

func (l nopLogger) Info(msg string, keysAndValues ...any)             {}
func (l nopLogger) Debug(msg string, keysAndValues ...any)            {}
func (l nopLogger) WithValues(keysAndValues ...any) Logger            { return nopLogger{} }
func (l nopLogger) WithName(name string) Logger                       { return nopLogger{} }
func (l nopLogger) Error(err error, msg string, keysAndValues ...any) {}

// NewLogrLogger returns a Logger that is satisfied by the supplied logr.Logger,
// which may be satisfied in turn by various logging implementations (Zap, klog,
// etc). Debug messages are logged at V(1).
func NewLogrLogger(l logr.Logger) Logger {
	return logrLogger{log: l}
}

type logrLogger struct {
	log logr.Logger
}

func (l logrLogger) Info(msg string, keysAndValues ...any) {
	l.log.Info(msg, keysAndValues...) //nolint:logrlint // False positive - logrlint thinks there's an odd number of args.
}

func (l logrLogger) Debug(msg string, keysAndValues ...any) {
	l.log.V(1).Info(msg, keysAndValues...) //nolint:logrlint // False positive - logrlint thinks there's an odd number of args.
}

func (l logrLogger) Error(err error, msg string, keysAndValues ...any) {
	l.log.Error(err, msg, keysAndValues...) //nolint:logrlint // False positive - logrlint thinks there's an odd number of args.
}

func (l logrLogger) WithName(name string) Logger {
	return logrLogger{log: l.log.WithName(name)}
}

func (l logrLogger) WithValues(keysAndValues ...any) Logger {
	return logrLogger{log: l.log.WithValues(keysAndValues...)} //nolint:logrlint // False positive - logrlint thinks there's an odd number of args.
}
