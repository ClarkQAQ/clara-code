package tool_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"clara-code/pkg/tool"
	"clara-code/pkg/tool/fs/sftp"
	"clara-code/pkg/util/errs"
	sftpproto "github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

func TestSSHCommandTreatsMetadataAsData(t *testing.T) {
	root := t.TempDir()
	dir := "-dir '$(printf injected)`printf injected`;\n中文"
	for _, name := range []string{dir, "-"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	value := "a=b '$(printf injected)`printf injected`;\n中文"
	for _, tc := range []struct {
		name string
		spec tool.Spec
		want string
		code int
	}{
		{
			name: "quoted metadata and multiline command",
			spec: tool.Spec{
				Workdir: dir,
				Env:     map[string]string{"CLARA_VALUE": value, "CLARA_EMPTY": ""},
				Command: "printf '%s|%s|%s' \"$PWD\" \"$CLARA_VALUE\" \"${CLARA_EMPTY-unset}\"\nexit 7",
			},
			want: filepath.Join(root, dir) + "|" + value + "|",
			code: 7,
		},
		{
			name: "literal hyphen directory",
			spec: tool.Spec{Workdir: "-", Command: "printf '%s' \"$PWD\""},
			want: filepath.Join(root, "-"),
		},
		{
			name: "missing directory stops the entire script",
			spec: tool.Spec{Workdir: "missing", Command: "false || printf unexpected\nprintf unexpected"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := sshTestRunner(t, root)
			process, err := runner.Start(t.Context(), tc.spec)
			if err != nil {
				t.Fatal(err)
			}
			defer process.Close()
			result, err := process.Poll(t.Context(), 3*time.Second)
			if err != nil || result.Running {
				t.Fatalf("SSH process did not finish: %+v, %v", result, err)
			}
			if tc.spec.Workdir == "missing" {
				if result.ExitCode == 0 || strings.Contains(result.Output, "unexpected") {
					t.Fatalf("command ran after cd failed: %+v", result)
				}
			} else if result.ExitCode != tc.code || result.Output != tc.want {
				t.Fatalf("got exit %d and output %q, want exit %d and output %q",
					result.ExitCode, result.Output, tc.code, tc.want)
			}
		})
	}
}

func TestSSHRejectsInvalidMetadataBeforeDial(t *testing.T) {
	runner, err := tool.NewSSH(tool.SSHConfig{
		Address: "no-port", User: "fixture", Password: "fixture", AllowInsecure: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, spec := range []tool.Spec{
		{Command: "true", Env: map[string]string{"OK; printf injected #": "value"}},
		{Command: "true", Env: map[string]string{"OK[0]": "value"}},
		{Command: "true", Env: map[string]string{"OK": "value\x00suffix"}},
		{Command: "true", Workdir: "dir\x00suffix"},
		{Command: "true\x00"},
	} {
		process, err := runner.Start(t.Context(), spec)
		if process != nil {
			process.Close()
		}
		if !errs.IsCode(err, errs.CodeInvalidArgument) {
			t.Fatalf("expected invalid_argument before dial, got %v", err)
		}
	}
}

func TestSSHCustomTransport(t *testing.T) {
	root := t.TempDir()
	config := sshTestConfig(t, func(channel ssh.Channel, requests <-chan *ssh.Request) {
		serveSSHTestCommand(t, root, channel, requests)
	})
	closed := make(chan struct{})
	config.DialContext = withoutDeadlines(t, config.Address, closed)
	runner, err := tool.NewSSH(config)
	if err != nil {
		t.Fatal(err)
	}
	process, err := runner.Start(t.Context(), tool.Spec{Command: "printf connected"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = process.Close() })
	result, err := process.Poll(t.Context(), time.Second)
	if err != nil || result.Running || result.Output != "connected" {
		t.Fatalf("SSH process did not finish through custom transport: %+v, %v", result, err)
	}
	if err := process.Close(); err != nil {
		t.Fatal(err)
	}
	assertTransportClosed(t, closed)
}

func TestSFTPCustomTransport(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "file"), []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	config := sshTestConfig(t, func(channel ssh.Channel, requests <-chan *ssh.Request) {
		serveSFTPTestSubsystem(channel, requests, func() {
			server, err := sftpproto.NewServer(channel, sftpproto.WithServerWorkingDirectory(root))
			if err == nil {
				defer server.Close()
				_ = server.Serve()
			}
		})
	})
	closed := make(chan struct{})
	files, err := sftp.Dial(t.Context(), sftp.Config{
		Address: config.Address, User: config.User, Password: config.Password,
		HostKeyCallback: config.HostKeyCallback, Timeout: config.Timeout,
		DialContext: withoutDeadlines(t, config.Address, closed),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = files.Close() })
	file, err := files.Stat(t.Context(), "/file")
	if err != nil || file.Size != 7 {
		t.Fatalf("SFTP Stat through custom transport: %+v, %v", file, err)
	}
	if err := files.Close(); err != nil {
		t.Fatal(err)
	}
	assertTransportClosed(t, closed)
}

func TestSFTPCancellationClosesCustomTransportDuringSubsystemSetup(t *testing.T) {
	started := make(chan struct{})
	config := sshTestConfig(t, func(channel ssh.Channel, requests <-chan *ssh.Request) {
		serveSFTPTestSubsystem(channel, requests, func() {
			close(started)
			_, _ = io.Copy(io.Discard, channel)
		})
	})
	closed := make(chan struct{})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := sftp.Dial(ctx, sftp.Config{
			Address: config.Address, User: config.User, Password: config.Password,
			HostKeyCallback: config.HostKeyCallback, Timeout: config.Timeout,
			DialContext: withoutDeadlines(t, config.Address, closed),
		})
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("SFTP subsystem did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) || errors.Is(err, sftp.ErrSSHHandshake) {
			t.Fatalf("SFTP subsystem cancellation returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("SFTP did not stop after cancellation")
	}
	assertTransportClosed(t, closed)
}

func TestSFTPHandshakeTimeoutClosesCustomTransport(t *testing.T) {
	connection, peer := net.Pipe()
	t.Cleanup(func() { _ = peer.Close() })
	closed := make(chan struct{})
	_, err := sftp.Dial(t.Context(), sftp.Config{
		Address: "fixture:22", User: "fixture", Password: "fixture",
		AllowInsecureHostKey: true, Timeout: 30 * time.Millisecond,
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			return &transportWithoutDeadlines{Conn: connection, closed: closed}, nil
		},
	})
	if !errors.Is(err, sftp.ErrSSHHandshake) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("SFTP handshake timeout returned %v", err)
	}
	assertTransportClosed(t, closed)
}

func serveSFTPTestSubsystem(channel ssh.Channel, requests <-chan *ssh.Request, serve func()) {
	defer channel.Close()
	for request := range requests {
		var payload struct{ Name string }
		if request.Type != "subsystem" || ssh.Unmarshal(request.Payload, &payload) != nil || payload.Name != "sftp" {
			_ = request.Reply(false, nil)
			continue
		}
		if request.Reply(true, nil) == nil {
			serve()
		}
		return
	}
}

type transportWithoutDeadlines struct {
	net.Conn
	closeOnce sync.Once
	closed    chan struct{}
}

func (c *transportWithoutDeadlines) SetDeadline(time.Time) error {
	return errors.ErrUnsupported
}

func (c *transportWithoutDeadlines) Close() error {
	var err error
	c.closeOnce.Do(func() {
		err = c.Conn.Close()
		close(c.closed)
	})
	return err
}

func withoutDeadlines(t *testing.T, address string, closed chan struct{}) func(context.Context, string, string) (net.Conn, error) {
	t.Helper()
	return func(ctx context.Context, network, target string) (net.Conn, error) {
		if network != "tcp" || target != address {
			t.Errorf("dialed %s %s, want tcp %s", network, target, address)
		}
		connection, err := (&net.Dialer{}).DialContext(ctx, network, target)
		if err != nil {
			return nil, err
		}
		return &transportWithoutDeadlines{Conn: connection, closed: closed}, nil
	}
}

func assertTransportClosed(t *testing.T, closed <-chan struct{}) {
	t.Helper()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("custom transport was not closed")
	}
}

func sshTestRunner(t *testing.T, root string) *tool.SSH {
	t.Helper()
	runner, err := tool.NewSSH(sshTestConfig(t, func(channel ssh.Channel, requests <-chan *ssh.Request) {
		serveSSHTestCommand(t, root, channel, requests)
	}))
	if err != nil {
		t.Fatal(err)
	}
	return runner
}

func sshTestConfig(t *testing.T, serve func(ssh.Channel, <-chan *ssh.Request)) tool.SSHConfig {
	t.Helper()
	listen := net.ListenConfig{}
	listener, err := listen.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{
		PasswordCallback: func(_ ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if string(password) == "fixture" {
				return nil, nil
			}
			return nil, errors.New("invalid fixture password")
		},
	}
	config.AddHostKey(signer)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		server, channels, requests, err := ssh.NewServerConn(conn, config)
		if err != nil {
			return
		}
		defer server.Close()
		go ssh.DiscardRequests(requests)
		for incoming := range channels {
			if incoming.ChannelType() != "session" {
				_ = incoming.Reject(ssh.UnknownChannelType, "expected session")
				continue
			}
			channel, requests, err := incoming.Accept()
			if err == nil {
				serve(channel, requests)
			}
		}
	}()
	return tool.SSHConfig{
		Address: listener.Addr().String(), User: "fixture", Password: "fixture",
		HostKeyCallback: ssh.FixedHostKey(signer.PublicKey()), Timeout: 3 * time.Second,
	}
}

func serveSSHTestCommand(t *testing.T, root string, channel ssh.Channel, requests <-chan *ssh.Request) {
	defer channel.Close()
	for request := range requests {
		var payload struct{ Command string }
		if request.Type != "exec" || ssh.Unmarshal(request.Payload, &payload) != nil {
			_ = request.Reply(false, nil)
			continue
		}
		_ = request.Reply(true, nil)
		command := exec.CommandContext(t.Context(), "/bin/sh", "-c", payload.Command)
		command.Dir = root
		command.Stdout = channel
		command.Stderr = channel.Stderr()
		var status uint32
		if err := command.Run(); err != nil {
			var exit *exec.ExitError
			status = 255
			if errors.As(err, &exit) && exit.ExitCode() >= 0 {
				status = uint32(exit.ExitCode())
			}
		}
		_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{status}))
		return
	}
}
