// Package sftp provides an SFTP-backed Clara Core filesystem.
package sftp

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	clarafs "clara-code/pkg/tools/fs"
	"clara-code/pkg/util/errs"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// Config describes one SSH/SFTP connection.
type Config struct {
	Address              string
	User                 string
	Password             string
	PrivateKey           []byte
	HostKeyCallback      ssh.HostKeyCallback
	AllowInsecureHostKey bool
	Root                 string
	Timeout              time.Duration
}

// FS maps logical paths below a remote SFTP root.
type FS struct {
	ssh       *ssh.Client
	client    *sftp.Client
	root      string
	closeOnce sync.Once
	closeErr  error
}

// Dial opens an SFTP filesystem.
func Dial(ctx context.Context, config Config) (*FS, error) {
	if strings.TrimSpace(config.Address) == "" || !strings.Contains(config.Address, ":") {
		return nil, errs.New(errs.CodeInvalidArgument, "SFTP address must include a host and port")
	}
	if config.User == "" {
		return nil, errs.New(errs.CodeInvalidArgument, "SFTP user is required")
	}
	if config.HostKeyCallback == nil {
		if !config.AllowInsecureHostKey {
			return nil, errs.New(errs.CodeForbidden, "SFTP host-key callback is required")
		}
		config.HostKeyCallback = ssh.InsecureIgnoreHostKey()
	}
	auth, err := authMethods(config)
	if err != nil {
		return nil, err
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	dialer := net.Dialer{Timeout: timeout}
	connection, err := dialer.DialContext(ctx, "tcp", config.Address)
	if err != nil {
		return nil, err
	}
	_ = connection.SetDeadline(time.Now().Add(timeout))
	sshConnection, channels, requests, err := ssh.NewClientConn(connection, config.Address, &ssh.ClientConfig{
		User: config.User, Auth: auth, HostKeyCallback: config.HostKeyCallback, Timeout: timeout,
	})
	if err != nil {
		connection.Close()
		return nil, err
	}
	_ = connection.SetDeadline(time.Time{})
	sshClient := ssh.NewClient(sshConnection, channels, requests)
	client, err := sftp.NewClient(sshClient)
	if err != nil {
		sshClient.Close()
		return nil, err
	}
	root := strings.TrimSpace(config.Root)
	if root == "" {
		root = "."
	}
	resolved, err := client.RealPath(root)
	if err != nil {
		client.Close()
		sshClient.Close()
		return nil, err
	}
	return &FS{ssh: sshClient, client: client, root: path.Clean(resolved)}, nil
}

func authMethods(config Config) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod
	if len(config.PrivateKey) > 0 {
		signer, err := ssh.ParsePrivateKey(config.PrivateKey)
		if err != nil {
			return nil, fmt.Errorf("parse SSH private key: %w", err)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}
	if config.Password != "" {
		methods = append(methods, ssh.Password(config.Password))
	}
	if len(methods) == 0 {
		return nil, errs.New(errs.CodeUnauthorized, "SFTP requires a password or private key")
	}
	return methods, nil
}

// Close closes the SFTP and SSH clients.
func (s *FS) Close() error {
	s.closeOnce.Do(func() { s.closeErr = errors.Join(s.client.Close(), s.ssh.Close()) })
	return s.closeErr
}

// ResolveDir maps a logical directory to its remote command path.
func (s *FS) ResolveDir(ctx context.Context, value clarafs.Path) (string, error) {
	target, err := s.remote(ctx, value, false)
	if err != nil {
		return "", err
	}
	info, err := s.client.Stat(target)
	if err != nil {
		return "", mapError(err)
	}
	if !info.IsDir() {
		return "", errs.New(errs.CodeInvalidArgument, "working directory is not a directory")
	}
	return target, nil
}

func (s *FS) remote(ctx context.Context, value clarafs.Path, allowFinalLink bool) (string, error) {
	clean, err := clarafs.Clean(value.String())
	if err != nil {
		return "", err
	}
	relative := strings.TrimPrefix(clean.String(), "/")
	if relative == "" {
		return s.root, ctx.Err()
	}
	parts := strings.Split(relative, "/")
	if len(parts) > 64 || len(relative) > 4096 {
		return "", errs.New(errs.CodeLimit, "remote path exceeds limits")
	}
	target := s.root
	for i, part := range parts {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		target = path.Join(target, part)
		info, err := s.client.Lstat(target)
		if errs.IsCode(mapError(err), errs.CodeNotFound) {
			return path.Join(target, strings.Join(parts[i+1:], "/")), nil
		}
		if err != nil {
			return "", mapError(err)
		}
		if info.Mode()&os.ModeSymlink != 0 && (!allowFinalLink || i != len(parts)-1) {
			return "", errs.ErrForbidden
		}
	}
	return target, nil
}

func (s *FS) Stat(ctx context.Context, value clarafs.Path) (clarafs.Entry, error) {
	if err := ctx.Err(); err != nil {
		return clarafs.Entry{}, err
	}
	target, err := s.remote(ctx, value, true)
	if err != nil {
		return clarafs.Entry{}, err
	}
	info, err := s.client.Lstat(target)
	if err != nil {
		return clarafs.Entry{}, mapError(err)
	}
	clean, _ := clarafs.Clean(value.String())
	return clarafs.Entry{
		Path: clean, Name: clean.Base(), Size: info.Size(), Mode: info.Mode(),
		ModTime: info.ModTime(), IsDir: info.IsDir(), IsLink: info.Mode()&os.ModeSymlink != 0,
	}, nil
}

func (s *FS) ReadDir(ctx context.Context, value clarafs.Path) ([]clarafs.Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	target, err := s.remote(ctx, value, false)
	if err != nil {
		return nil, err
	}
	items, err := s.client.ReadDirContext(ctx, target)
	if err != nil {
		return nil, mapError(err)
	}
	if len(items) > 10000 {
		return nil, errs.New(errs.CodeLimit, "remote directory has too many entries")
	}
	out := make([]clarafs.Entry, 0, len(items))
	for _, info := range items {
		child, err := clarafs.Join(value, info.Name())
		if err != nil {
			continue
		}
		out = append(out, clarafs.Entry{
			Path: child, Name: info.Name(), Size: info.Size(), Mode: info.Mode(),
			ModTime: info.ModTime(), IsDir: info.IsDir(), IsLink: info.Mode()&os.ModeSymlink != 0,
		})
	}
	return out, nil
}

func (s *FS) Open(ctx context.Context, value clarafs.Path) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	target, err := s.remote(ctx, value, false)
	if err != nil {
		return nil, err
	}
	file, err := s.client.Open(target)
	if err != nil {
		return nil, mapError(err)
	}
	return file, nil
}

func (s *FS) ReadFile(ctx context.Context, value clarafs.Path, max int64) ([]byte, error) {
	file, err := s.Open(ctx, value)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if max <= 0 || max > 64<<20 {
		max = 64 << 20
	}
	data, err := io.ReadAll(io.LimitReader(file, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		data = data[:max]
	}
	return data, nil
}

func (s *FS) ReadAt(ctx context.Context, value clarafs.Path, offset, limit int64) ([]byte, error) {
	if offset < 0 || limit < 0 || limit > 64<<20 {
		return nil, errs.New(errs.CodeInvalidArgument, "negative read range")
	}
	target, err := s.remote(ctx, value, false)
	if err != nil {
		return nil, err
	}
	file, err := s.client.Open(target)
	if err != nil {
		return nil, mapError(err)
	}
	defer file.Close()
	buffer := make([]byte, limit)
	read, err := file.ReadAt(buffer, offset)
	if errors.Is(err, io.EOF) {
		err = nil
	}
	if err != nil {
		return nil, err
	}
	return buffer[:read], nil
}

func (s *FS) WriteFile(ctx context.Context, value clarafs.Path, data []byte, options clarafs.WriteOptions) (clarafs.Entry, error) {
	if err := ctx.Err(); err != nil {
		return clarafs.Entry{}, err
	}
	target, err := s.remote(ctx, value, false)
	if err != nil {
		return clarafs.Entry{}, err
	}
	if !options.Overwrite {
		if _, err := s.client.Lstat(target); !errs.IsCode(mapError(err), errs.CodeNotFound) {
			if err == nil {
				err = errs.ErrConflict
			}
			return clarafs.Entry{}, mapError(err)
		}
	}
	if options.CreateDirs {
		if err := s.client.MkdirAll(path.Dir(target)); err != nil {
			return clarafs.Entry{}, mapError(err)
		}
	}
	temp := path.Join(path.Dir(target), ".clara-core-write-"+rand.Text())
	file, err := s.client.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		return clarafs.Entry{}, mapError(err)
	}
	_, writeErr := file.Write(data)
	mode := options.Mode
	if mode == 0 {
		mode = 0o644
	}
	if writeErr == nil {
		writeErr = file.Chmod(mode)
	}
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return clarafs.Entry{}, errors.Join(err, s.client.Remove(temp))
	}
	if options.Overwrite {
		if _, ok := s.client.HasExtension("posix-rename@openssh.com"); ok {
			err = s.client.PosixRename(temp, target)
		} else {
			err = s.client.Rename(temp, target)
		}
	} else {
		err = s.client.Rename(temp, target)
	}
	if err != nil {
		return clarafs.Entry{}, errors.Join(mapError(err), s.client.Remove(temp))
	}
	return s.Stat(ctx, value)
}

func (s *FS) MkdirAll(ctx context.Context, value clarafs.Path, mode fs.FileMode) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	target, err := s.remote(ctx, value, false)
	if err != nil {
		return err
	}
	return s.client.MkdirAll(target)
}

func (s *FS) Remove(ctx context.Context, value clarafs.Path, options clarafs.RemoveOptions) error {
	if err := clarafs.EnsureNotRoot(value); err != nil {
		return err
	}
	target, err := s.remote(ctx, value, true)
	if err != nil {
		return err
	}
	info, err := s.client.Lstat(target)
	if err != nil {
		return mapError(err)
	}
	if !info.IsDir() {
		return mapError(s.client.Remove(target))
	}
	if !options.Recursive {
		return mapError(s.client.RemoveDirectory(target))
	}
	type entry struct {
		name      string
		directory bool
	}
	var paths []entry
	walk := s.client.Walk(target)
	for walk.Step() {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := walk.Err(); err != nil {
			return mapError(err)
		}
		if len(paths) >= 10000 {
			return errs.New(errs.CodeLimit, "remote deletion walk exceeds the entry limit")
		}
		paths = append(paths, entry{walk.Path(), walk.Stat().IsDir()})
	}
	for i := len(paths) - 1; i >= 0; i-- {
		if err := ctx.Err(); err != nil {
			return err
		}
		item := paths[i]
		if item.directory {
			err = s.client.RemoveDirectory(item.name)
		} else {
			err = s.client.Remove(item.name)
		}
		if err != nil {
			return mapError(err)
		}
	}
	return nil
}

func (s *FS) Rename(ctx context.Context, from, to clarafs.Path) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := errors.Join(clarafs.EnsureNotRoot(from), clarafs.EnsureNotRoot(to)); err != nil {
		return err
	}
	source, err := s.remote(ctx, from, true)
	if err != nil {
		return err
	}
	target, err := s.remote(ctx, to, false)
	if err != nil {
		return err
	}
	if err := s.client.MkdirAll(path.Dir(target)); err != nil {
		return err
	}
	return s.client.Rename(source, target)
}

func mapError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, os.ErrNotExist) || strings.Contains(strings.ToLower(err.Error()), "not exist") {
		return errs.Wrap(errs.CodeNotFound, "remote path not found", err)
	}
	if strings.Contains(strings.ToLower(err.Error()), "permission") {
		return errs.Wrap(errs.CodeForbidden, "remote permission denied", err)
	}
	return err
}

var _ clarafs.FS = (*FS)(nil)
