package host

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"clara-code/pkg/util/errs"
)

func resolveNative(cwd, value string) (string, error) {
	value = filepath.FromSlash(value)
	if value == "~" || strings.HasPrefix(value, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		value = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(value, "~"), `\`))
	}

	if strings.HasPrefix(value, `\\?\`) || strings.HasPrefix(value, `\\.\`) {
		return "", errs.New(errs.CodeInvalidArgument, "Windows device paths are not supported")
	}
	volume := filepath.VolumeName(value)
	if len(volume) == 2 && !strings.ContainsAny(volume[:1], "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ") {
		return "", errs.New(errs.CodeInvalidArgument, "invalid Windows drive")
	}
	if volume != "" && !filepath.IsAbs(value) {
		return "", errs.New(errs.CodeInvalidArgument, "drive-relative paths are not supported; use a drive root or a workspace-relative path")
	}
	if strings.HasPrefix(value, `\\`) {
		parts := strings.Split(strings.TrimPrefix(volume, `\\`), `\`)
		if len(parts) != 2 || !validComponent(parts[0]) || !validComponent(parts[1]) {
			return "", errs.New(errs.CodeInvalidArgument, "UNC paths require a server and a share")
		}
	}
	for _, part := range strings.Split(strings.TrimPrefix(value, volume), `\`) {
		if part != "" && part != "." && part != ".." && !validComponent(part) {
			return "", errs.New(errs.CodeInvalidArgument, "invalid Windows path component: "+part)
		}
	}

	switch {
	case volume != "":
	case strings.HasPrefix(value, `\`):
		value = filepath.VolumeName(cwd) + value
	default:
		value = filepath.Join(cwd, value)
	}
	value = filepath.Clean(value)
	volume = filepath.VolumeName(value)
	if len(volume) > 2 && strings.TrimRight(value, `\`) == volume {
		return volume, nil
	}
	if len(value) >= 2 && value[1] == ':' {
		value = strings.ToUpper(value[:1]) + value[1:]
	}
	return value, nil
}

func validComponent(part string) bool {
	return part != "" && part != "." && part != ".." &&
		!strings.ContainsAny(part, `<>:"|?*`) && !strings.ContainsFunc(part, func(r rune) bool { return r < 32 }) && !strings.HasSuffix(part, " ") &&
		!strings.HasSuffix(part, ".") && filepath.IsLocal(part)
}

func isLink(info os.FileInfo) bool {
	data, _ := info.Sys().(*syscall.Win32FileAttributeData)
	return info.Mode()&os.ModeSymlink != 0 ||
		data != nil && data.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0
}
