package session

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"

	"clara-code/pkg/model"
)

type Failure struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

func DescribeFailure(err error) Failure {
	failure := Failure{Kind: "error", Message: "The request failed"}
	if err == nil {
		return failure
	}
	failure.Message = err.Error()
	var provider *model.ProviderError
	var network net.Error
	switch {
	case errors.Is(err, context.Canceled):
		failure.Kind, failure.Message = "cancelled", "Interrupted"
	case errors.Is(err, context.DeadlineExceeded):
		failure.Kind = "timeout"
	case errors.As(err, &provider):
		switch {
		case provider.StatusCode == 401 || provider.StatusCode == 403 || provider.AuthError:
			failure.Kind = "authentication"
		case provider.StatusCode == 429:
			failure.Kind = "rate_limit"
		case provider.ContextTooLargeErr:
			failure.Kind = "context"
		case provider.StatusCode >= 500:
			failure.Kind = "server"
		case provider.TransientError || errors.Is(provider.Cause, io.ErrUnexpectedEOF) || model.IsTransportError(provider.Cause):
			failure.Kind = "stream"
		}
	case errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || model.IsTransportError(err):
		failure.Kind = "connection"
	case errors.As(err, &network):
		failure.Kind = "connection"
	}
	failure.Message = excerpt(strings.Join(strings.Fields(failure.Message), " "), 600)
	return failure
}
