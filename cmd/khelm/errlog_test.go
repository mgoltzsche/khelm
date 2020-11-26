package main

import (
	"fmt"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestTraceableRootCause(t *testing.T) {
	simpleErr := fmt.Errorf("simple error")
	traceableErr := errors.New("traceable error")
	wrapped := errors.Wrap(simpleErr, "wrapped")
	msgErr := errors.WithMessage(simpleErr, "msg")
	wrappedMsg := errors.Wrap(msgErr, "wrapped")
	tests := []struct {
		name     string
		input    error
		expected error
	}{
		{"no wrapping or causing", simpleErr, simpleErr},
		{"wrapping", fmt.Errorf("wrapped: %w", traceableErr), traceableErr},
		{"causing", errors.Wrap(traceableErr, "wrapped"), traceableErr},
		{"deeply nested", errors.Wrap(fmt.Errorf("errorf: %w", errors.Wrap(traceableErr, "wrappedinner")), "wrappedouter"), traceableErr},
		{"root cause without stack trace", errors.Wrap(wrapped, "wrappedouter"), wrapped},
		{"root cause without stack trace but formatter", errors.Wrap(wrappedMsg, "wrappedouter"), wrappedMsg},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := traceableRootCause(tt.input)
			require.Equal(t, tt.expected, err)
		})
	}
}
