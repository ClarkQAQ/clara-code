package sshclient

import (
	"context"
	"errors"
	"fmt"
	"net"

	"golang.org/x/crypto/ssh"
)

var ErrHandshake = errors.New("SSH handshake failed")

// Open dials an SSH client and bounds the handshake and initialization together.
func Open[T any](ctx context.Context, address string, dial func(context.Context, string, string) (net.Conn, error), config *ssh.ClientConfig, initialize func(*ssh.Client) (T, error)) (T, error) {
	var zero T
	ctx, cancel := context.WithTimeout(ctx, config.Timeout)
	defer cancel()
	if dial == nil {
		dialer := net.Dialer{}
		dial = dialer.DialContext
	}
	connection, err := dial(ctx, "tcp", address)
	if err != nil {
		return zero, setupError(ctx, err)
	}
	// SSH channel connections may not support deadlines.
	stopClose := context.AfterFunc(ctx, func() { _ = connection.Close() })
	defer stopClose()
	transport, channels, requests, err := ssh.NewClientConn(connection, address, config)
	if err != nil {
		return zero, fmt.Errorf("%w: %w", ErrHandshake, setupError(ctx, err))
	}
	client := ssh.NewClient(transport, channels, requests)
	result, err := initialize(client)
	if err != nil {
		_ = client.Close()
		return zero, setupError(ctx, err)
	}
	stopClose()
	if err := ctx.Err(); err != nil {
		_ = client.Close()
		return zero, err
	}
	return result, nil
}

func setupError(ctx context.Context, err error) error {
	if canceled := ctx.Err(); canceled != nil {
		return errors.Join(canceled, err)
	}
	return err
}
