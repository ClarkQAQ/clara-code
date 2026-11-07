package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"clara-code/pkg/model"

	"clara-code/internal/coding"
	"clara-code/internal/setting"
	"clara-code/internal/terminal"
)

type arguments struct {
	cwd, command, id    string
	demo, help, version bool
}

func parse(args []string, stderr io.Writer) (arguments, error) {
	var options arguments
	flags := flag.NewFlagSet("clara-code", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {}
	flags.StringVar(&options.cwd, "cwd", ".", "working directory")
	flags.BoolVar(&options.demo, "demo", false, "use the built-in demo model")
	flags.BoolVar(&options.help, "help", false, "show usage")
	flags.BoolVar(&options.version, "version", false, "show version")
	var rest []string
	for len(args) > 0 {
		arg := args[0]
		args = args[1:]
		if arg == "--" {
			rest = append(rest, args...)
			break
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			items := []string{arg}
			if (arg == "--cwd" || arg == "-cwd") && len(args) > 0 {
				items = append(items, args[0])
				args = args[1:]
			}
			if err := flags.Parse(items); errors.Is(err, flag.ErrHelp) {
				options.help = true
			} else if err != nil {
				return options, err
			}
			continue
		}
		rest = append(rest, arg)
	}
	if options.help || options.version || len(rest) == 0 {
		return options, nil
	}
	options.command = rest[0]
	switch options.command {
	case "acp":
		if len(rest) == 1 {
			return options, nil
		}
	case "resume":
		if len(rest) <= 2 {
			if len(rest) == 2 {
				options.id = rest[1]
			}
			return options, nil
		}
	}
	return options, fmt.Errorf("unexpected arguments %q (see --help)", rest)
}

// Run starts the CLI with process-owned streams and a build version.
func Run(ctx context.Context, args []string, input, output, stderr *os.File, version string) error {
	if len(args) > 0 && args[0] == "__acp_exec" {
		return coding.ExecACP(args[1:])
	}
	options, err := parse(args, stderr)
	if err != nil {
		return err
	}
	if options.version {
		_, err := fmt.Fprintln(output, "Clara Code "+version)
		return err
	}
	if options.help {
		_, err := fmt.Fprint(output, "Clara Code\n\nUsage:\n  clara-code [--cwd DIR] [--demo]\n  clara-code resume [ID] [--cwd DIR] [--demo]\n  clara-code acp [--demo]\n  clara-code --version\n")
		return err
	}
	config, err := setting.Load()
	if err != nil {
		return err
	}
	host := coding.Host{Config: config}
	if options.demo {
		host.Config.DefaultModel = "demo"
		host.Config.Models = map[string]setting.Model{"demo": {ID: "demo"}}
		host.Models = func(context.Context, string) (model.LanguageModel, error) { return coding.DemoModel{}, nil }
	} else {
		host.Models, err = config.Factory()
		if err != nil {
			return err
		}
	}
	if options.command == "acp" {
		slog.SetDefault(diagnosticLogger(stderr))
		return host.ServeACP(ctx, input, output, version)
	}
	workspace, err := coding.ResolveWorkspace(options.cwd)
	if err != nil {
		return err
	}
	// MCP and ACP SDK diagnostics must not write over the inline composer.
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	return terminal.Run(ctx, terminal.Options{
		Input: input, Output: output, Host: host, Workspace: workspace, Version: version,
		Resume: options.id, PickSession: options.command == "resume" && options.id == "",
	})
}
