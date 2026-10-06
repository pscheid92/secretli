// Package cli is the secretli command: share, open, status and delete.
//
// stdout carries the result and nothing else, so `secretli share … | pbcopy`
// copies exactly the link; everything said to the person goes to stderr
// when stdout is not a terminal. With --json the result is JSON, errors
// included. Nothing is ever asked when stdin is not a terminal.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/pscheid92/secretli/internal/share"
	"github.com/pscheid92/secretli/internal/share/api"
)

// Version is set at build time; otherwise the module version is used.
var Version = ""

// DefaultServer is where secrets go unless --server or SECRETLI_SERVER says otherwise.
const DefaultServer = "https://secretli.app"

// Exit codes scripts can branch on.
const (
	ExitOK       = 0
	ExitError    = 1
	ExitUsage    = 2
	ExitPassword = 3
	ExitGone     = 4
	ExitServer   = 5
)

type env struct {
	stdin  *os.File
	stdout io.Writer
	stderr io.Writer

	stdinTTY  bool
	stdoutTTY bool
	stderrTTY bool

	server string
	json   bool
	quiet  bool
}

// runError marks an error from a command's work, as opposed to one from
// parsing the command line.
type runError struct{ err error }

func (e *runError) Error() string { return e.err.Error() }
func (e *runError) Unwrap() error { return e.err }

// Main runs the command line and returns the exit code.
func Main(args []string, stdin, stdout, stderr *os.File) int {
	e := &env{
		stdin: stdin, stdout: stdout, stderr: stderr,
		stdinTTY:  term.IsTerminal(int(stdin.Fd())),
		stdoutTTY: term.IsTerminal(int(stdout.Fd())),
		stderrTTY: term.IsTerminal(int(stderr.Fd())),
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	root := newRoot(e)
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	if err := root.ExecuteContext(ctx); err != nil {
		return e.report(err)
	}
	return ExitOK
}

func newRoot(e *env) *cobra.Command {
	root := &cobra.Command{
		Use:   "secretli",
		Short: "Share secrets that are encrypted before they leave your machine",
		Long: `Secretli shares text and files through links that only the holder can open.
Everything is encrypted here, in this command; the server stores ciphertext
and never sees the key, which travels in the part of the link after #.

Links from this command open in the web app and the other way round.`,
		Example: `  secretli share                       type or paste a secret, then Ctrl-D
  pbpaste | secretli share -e 1h       text from a pipe, gone after an hour
  secretli share deploy.key notes.pdf  one or more files
  secretli open <link>                 text to stdout, files to the current directory
  secretli status <link>               what a link points to, without opening it
  secretli delete <owner-link>         remove a secret for everyone`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version(),
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			e.server = normalizeServer(e.server)
			return nil
		},
	}
	root.SetVersionTemplate("secretli {{.Version}}\n")
	defaultServer := os.Getenv("SECRETLI_SERVER")
	if defaultServer == "" {
		defaultServer = DefaultServer
	}
	root.PersistentFlags().StringVar(&e.server, "server", defaultServer, "the Secretli server for new secrets (also SECRETLI_SERVER)")
	root.PersistentFlags().BoolVar(&e.json, "json", false, "print results and errors as JSON")
	root.PersistentFlags().BoolVarP(&e.quiet, "quiet", "q", false, "print only the result")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return err })

	root.AddCommand(newShareCmd(e), newOpenCmd(e), newStatusCmd(e), newDeleteCmd(e))
	return root
}

func version() string {
	if Version != "" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}

func normalizeServer(s string) string {
	s = strings.TrimRight(strings.TrimSpace(s), "/")
	if s != "" && !strings.Contains(s, "://") {
		s = "https://" + s
	}
	return s
}

func (e *env) client(origin string) *api.Client {
	c := api.New(origin)
	c.HTTP = &http.Client{}
	c.UserAgent = "secretli-cli/" + version()
	return c
}

// say talks to the person: stderr when stdout is a pipe, stdout otherwise.
// Silent with --quiet or --json.
func (e *env) say(format string, args ...any) {
	if e.quiet || e.json {
		return
	}
	w := e.stdout
	if !e.stdoutTTY {
		w = e.stderr
	}
	_, _ = fmt.Fprintf(w, format, args...)
}

// note goes to stderr whatever the mode, unless quiet: warnings and progress.
func (e *env) note(format string, args ...any) {
	if e.quiet {
		return
	}
	_, _ = fmt.Fprintf(e.stderr, format, args...)
}

func (e *env) emitJSON(v any) error {
	enc := json.NewEncoder(e.stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// report prints an error and picks its exit code.
func (e *env) report(err error) int {
	var silent *exitWith
	if errors.As(err, &silent) {
		// The command printed its result already; only the code is left.
		return silent.code
	}
	code := exitCode(err)
	var gone *share.GoneError
	if e.json {
		body := map[string]any{"error": message(err), "code": code}
		if errors.As(err, &gone) {
			body["outcome"] = gone.Gone.Outcome
			body["ended_at"] = gone.Gone.EndedAt
			body["opened_by_owner"] = gone.Gone.OpenedByOwner
			if gone.Gone.FirstOpenedAt != nil {
				body["first_opened_at"] = *gone.Gone.FirstOpenedAt
			}
		}
		_ = e.emitJSON(body)
		return code
	}
	_, _ = fmt.Fprintf(e.stderr, "secretli: %s\n", message(err))
	if code == ExitUsage {
		_, _ = fmt.Fprintf(e.stderr, "Run 'secretli --help' for usage.\n")
	}
	return code
}

func exitCode(err error) int {
	var run *runError
	if !errors.As(err, &run) {
		return ExitUsage
	}
	var gone *share.GoneError
	var notFound *share.NotFoundError
	var apiErr *api.Error
	switch {
	case errors.Is(err, share.ErrPasswordRequired), errors.Is(err, share.ErrWrongPassword):
		return ExitPassword
	case errors.As(err, &gone), errors.As(err, &notFound):
		return ExitGone
	case errors.As(err, &apiErr) && (apiErr.Status == 0 || apiErr.Status >= 500 || apiErr.Status == http.StatusTooManyRequests):
		return ExitServer
	case errors.Is(err, context.Canceled):
		return ExitError
	}
	return ExitError
}

func message(err error) string {
	var gone *share.GoneError
	if errors.As(err, &gone) {
		return goneSentence(gone.Gone, false)
	}
	if errors.Is(err, context.Canceled) {
		return "interrupted"
	}
	return err.Error()
}

// run wraps a command's work so its errors are told apart from usage errors.
func run(fn func(cmd *cobra.Command, args []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if err := fn(cmd, args); err != nil {
			return &runError{err: err}
		}
		return nil
	}
}
