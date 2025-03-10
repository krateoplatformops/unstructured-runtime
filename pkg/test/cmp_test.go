package test

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestEquateErrors(t *testing.T) {
	tests := []struct {
		name string
		a    error
		b    error
		want bool
	}{
		{
			name: "BothNil",
			a:    nil,
			b:    nil,
			want: true,
		},
		{
			name: "OneNil",
			a:    errors.New("error"),
			b:    nil,
			want: false,
		},
		{
			name: "DifferentTypes",
			a:    errors.New("error"),
			b:    &customError{"error"},
			want: false,
		},
		{
			name: "SameError",
			a:    errors.New("error"),
			b:    errors.New("error"),
			want: true,
		},
		{
			name: "DifferentErrorStrings",
			a:    errors.New("error1"),
			b:    errors.New("error2"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cmp.Equal(tt.a, tt.b, EquateErrors()); got != tt.want {
				t.Errorf("EquateErrors() = %v, want %v", got, tt.want)
			}
		})
	}
}

type customError struct {
	msg string
}

func (e *customError) Error() string {
	return e.msg
}
