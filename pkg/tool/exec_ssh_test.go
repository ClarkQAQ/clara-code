package tool_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"clara-code/pkg/tool"
	"clara-code/pkg/util/errs"
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

func sshTestRunner(t *testing.T, root string) *tool.SSH {
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
				serveSSHTestCommand(t, root, channel, requests)
			}
		}
	}()
	runner, err := tool.NewSSH(tool.SSHConfig{
		Address: listener.Addr().String(), User: "fixture", Password: "fixture",
		HostKeyCallback: ssh.FixedHostKey(signer.PublicKey()), Timeout: 3 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	return runner
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
