package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/pscheid92/secretli/internal/share"
	"github.com/secretli/format/bundle"
)

type openOptions struct {
	out          string
	toStdout     bool
	force        bool
	password     bool
	passwordFile string
}

func newOpenCmd(e *env) *cobra.Command {
	var o openOptions
	cmd := &cobra.Command{
		Use:   "open [link]",
		Short: "Open a secret: text to stdout, files to disk",
		Long: `Opens a link. Text is written to stdout exactly as it was shared; files are
saved to the current directory, or to --out, and never overwrite anything
unless --force is given.

Opening a one-time secret is what uses it up, so the link is described first.
Without an argument the link is read from stdin, or asked for at a terminal,
which keeps it out of your shell history.`,
		Example: `  secretli open 'https://secretli.app/s#…'
  secretli open 'https://secretli.app/s#…' --out ./received
  secretli open 'https://secretli.app/s#…' --stdout > backup.tgz
  pbpaste | secretli open`,
		Args: cobra.MaximumNArgs(1),
		RunE: run(func(cmd *cobra.Command, args []string) error {
			return e.open(cmd.Context(), o, args)
		}),
	}
	f := cmd.Flags()
	f.StringVarP(&o.out, "out", "o", ".", "the directory files are saved to")
	f.BoolVar(&o.toStdout, "stdout", false, "write a single file to stdout instead of saving it")
	f.BoolVar(&o.force, "force", false, "overwrite files that already exist")
	f.BoolVarP(&o.password, "password", "p", false, "the secret has a password (asked for, or SECRETLI_PASSWORD)")
	f.StringVar(&o.passwordFile, "password-file", "", "read the password from the first line of this file")
	return cmd
}

// describedGone carries the owner's or the recipient's version of what became
// of a secret, while staying a GoneError for the exit code.
type describedGone struct {
	inner *share.GoneError
	owner bool
}

func (d *describedGone) Error() string { return goneSentence(d.inner.Gone, d.owner) }
func (d *describedGone) Unwrap() error { return d.inner }

func describeGone(err error, owner bool) error {
	var gone *share.GoneError
	if errors.As(err, &gone) {
		return &describedGone{inner: gone, owner: owner}
	}
	return err
}

func (e *env) open(ctx context.Context, o openOptions, args []string) error {
	link, err := e.linkArg(args)
	if err != nil {
		return err
	}
	c := e.client(link.Origin)
	now := time.Now()
	info, err := share.Inspect(ctx, c, link)
	if err != nil {
		return describeGone(err, link.IsOwner())
	}
	e.say("%s\n", describeInfo(info, link.IsOwner(), now))

	source := passwordSource{flag: o.password, file: o.passwordFile}
	password, err := source.resolve(e, info.PasswordProtected, false)
	if err != nil {
		if info.PasswordProtected {
			return fmt.Errorf("%w: %w", share.ErrPasswordRequired, err)
		}
		return err
	}

	sink := &fileSink{e: e, dir: o.out, toStdout: o.toStdout, force: o.force}
	bar := e.progress("Downloading and decrypting…")
	for attempt := 1; ; attempt++ {
		opened, err := share.Open(ctx, c, link, password, sink, bar.update)
		bar.finish()
		if err == nil {
			return e.printOpened(opened, sink)
		}
		// A mistyped password costs nothing: the server only hands out the
		// secret once the right token arrives. Ask again at a terminal.
		if errors.Is(err, share.ErrWrongPassword) && source.file == "" && os.Getenv("SECRETLI_PASSWORD") == "" && attempt < 3 {
			if t, terr := e.terminal(); terr == nil {
				e.note("Wrong password. Try again.\n")
				password, err = t.askSecret("Password: ")
				t.close()
				if err == nil && password != "" {
					continue
				}
			}
		}
		return describeGone(err, link.IsOwner())
	}
}

// linkArg takes the link from the arguments, from stdin, or from a prompt.
func (e *env) linkArg(args []string) (share.Link, error) {
	raw := ""
	switch {
	case len(args) == 1:
		raw = args[0]
	case !e.stdinTTY:
		line, err := bufio.NewReader(e.stdin).ReadString('\n')
		if err != nil && (!errors.Is(err, io.EOF) || line == "") {
			return share.Link{}, errors.New("no link given, and nothing on stdin")
		}
		raw = line
	default:
		t, err := e.terminal()
		if err != nil {
			return share.Link{}, errors.New("no link given")
		}
		defer t.close()
		if raw, err = t.askLine("Paste the link: "); err != nil {
			return share.Link{}, err
		}
	}
	link, err := share.ParseLink(raw)
	if err != nil {
		return share.Link{}, fmt.Errorf("%w: expected https://host/s#… as the web app prints it", err)
	}
	return link, nil
}

// fileSink writes a text secret to stdout and files to a directory, or a
// single file to stdout.
type fileSink struct {
	e        *env
	dir      string
	toStdout bool
	force    bool

	text  []byte
	saved []savedFile
}

type savedFile struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	Path string `json:"path,omitempty"`
}

func (s *fileSink) Manifest(m *bundle.Manifest) error {
	if s.toStdout {
		if len(m.Files) != 1 {
			return fmt.Errorf("--stdout is for a single file, and this secret has %d", len(m.Files))
		}
		return nil
	}
	if len(m.Files) == 1 && m.Files[0].Name == "secret.txt" && m.BundleName == "secret.txt" {
		return nil // text, which goes to stdout
	}
	if err := os.MkdirAll(s.dir, 0o750); err != nil {
		return fmt.Errorf("create %s: %w", s.dir, err)
	}
	if !s.force {
		for _, f := range m.Files {
			path := filepath.Join(s.dir, safeName(f.Name))
			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf("%s exists; use --force to overwrite, or --out for another directory", path)
			}
		}
	}
	return nil
}

func (s *fileSink) Text(text []byte) error {
	s.text = text
	return nil
}

func (s *fileSink) File(f bundle.File) (io.WriteCloser, error) {
	if s.toStdout {
		s.saved = append(s.saved, savedFile{Name: f.Name, Size: f.Size})
		return nopWriteCloser{s.e.stdout}, nil
	}
	path := filepath.Join(s.dir, safeName(f.Name))
	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if !s.force {
		flags |= os.O_EXCL
	}
	file, err := os.OpenFile(path, flags, 0o600) //nolint:gosec // the name is sanitised and confined to the output directory
	if err != nil {
		return nil, err
	}
	s.saved = append(s.saved, savedFile{Name: f.Name, Size: f.Size, Path: path})
	return file, nil
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

// safeName keeps a shared file's name from leaving the output directory.
func safeName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = filepath.Base(filepath.Clean("/" + name))
	if name == "" || name == "/" || name == "." {
		return "file"
	}
	return name
}

func (e *env) printOpened(opened *share.Opened, sink *fileSink) error {
	if e.json {
		out := map[string]any{
			"kind":       opened.Info.Kind,
			"reusable":   opened.Info.Reusable,
			"password":   opened.Info.PasswordProtected,
			"expires_at": opened.Info.ExpiresAt.UTC(),
			"created_at": opened.Info.CreatedAt.UTC(),
		}
		if opened.Info.Kind == share.KindText && sink.text != nil {
			out["text"] = string(sink.text)
		} else {
			out["files"] = sink.saved
		}
		return e.emitJSON(out)
	}
	if opened.Info.Kind == share.KindText && sink.text != nil {
		if _, err := e.stdout.Write(sink.text); err != nil {
			return err
		}
		if e.stdoutTTY && len(sink.text) > 0 && sink.text[len(sink.text)-1] != '\n' {
			_, _ = fmt.Fprintln(e.stdout)
		}
		return nil
	}
	if sink.toStdout {
		return nil
	}
	names := make([]string, 0, len(sink.saved))
	for _, f := range sink.saved {
		names = append(names, fmt.Sprintf("%s (%s)", f.Name, formatSize(f.Size)))
	}
	e.say("Saved %s to %s.\n", strings.Join(names, ", "), sink.dir)
	return nil
}
