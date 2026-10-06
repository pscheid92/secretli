package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/pscheid92/secretli/internal/share"
	"github.com/pscheid92/secretli/internal/share/bundle"
)

type shareOptions struct {
	text         string
	name         string
	expires      string
	passwordFile string
	reusable     bool
	password     bool
	qr           bool
	copyLink     bool
}

func newShareCmd(e *env) *cobra.Command {
	var o shareOptions
	cmd := &cobra.Command{
		Use:   "share [files...]",
		Short: "Encrypt text or files and get a link",
		Long: `Encrypts text or files here and uploads only the ciphertext. Prints the link
to hand out, and the owner link that can delete the secret and later tell
whether it was opened.

Files are given as arguments. Text comes from stdin, from --text, or, at a
terminal, from a prompt, which keeps it out of your shell history. "-" shares
stdin as a file named by --name.

Links open once unless --reusable is set, and expire after a day unless
--expires says otherwise.`,
		Example: `  secretli share                          type or paste, then Ctrl-D
  pbpaste | secretli share                text from a pipe
  secretli share -t "hunter2"             text as an argument (lands in shell history)
  secretli share deploy.key notes.pdf     files
  pg_dump db | gzip | secretli share --name db.sql.gz
  secretli share report.pdf -e 4h --reusable -p --qr`,
		Args: cobra.ArbitraryArgs,
		RunE: run(func(cmd *cobra.Command, args []string) error {
			return e.share(cmd.Context(), o, args)
		}),
	}
	f := cmd.Flags()
	f.StringVarP(&o.text, "text", "t", "", "the text to share, instead of reading it from stdin")
	f.StringVarP(&o.name, "name", "n", "", "share stdin as a file with this name, instead of as text")
	f.StringVarP(&o.expires, "expires", "e", share.DefaultExpiration, "how long the link lives: "+strings.Join(share.Expirations, ", "))
	f.BoolVar(&o.reusable, "reusable", false, "let the link open again and again until it expires")
	f.BoolVarP(&o.password, "password", "p", false, "protect the secret with a password (asked for, or SECRETLI_PASSWORD)")
	f.StringVar(&o.passwordFile, "password-file", "", "read the password from the first line of this file")
	f.BoolVar(&o.qr, "qr", false, "show the link as a QR code too")
	f.BoolVarP(&o.copyLink, "copy", "c", false, "copy the link to the clipboard")
	return cmd
}

func (e *env) share(ctx context.Context, o shareOptions, args []string) error {
	if !share.ValidExpiration(o.expires) {
		return fmt.Errorf("--expires must be one of %s", strings.Join(share.Expirations, ", "))
	}
	params := share.Params{Expiration: o.expires, Reusable: o.reusable}

	var closers []io.Closer
	defer func() {
		for _, c := range closers {
			_ = c.Close()
		}
	}()
	for _, arg := range args {
		if arg == "-" {
			data, err := e.readStdin()
			if err != nil {
				return err
			}
			name := o.name
			if name == "" {
				name = "stdin"
			}
			params.Files = append(params.Files, memorySource(name, data))
			continue
		}
		src, f, err := fileSource(arg)
		if err != nil {
			return err
		}
		closers = append(closers, f)
		params.Files = append(params.Files, src)
	}
	if o.text != "" && len(params.Files) > 0 {
		return errors.New("share text or files, not both at once")
	}
	if len(params.Files) == 0 {
		switch {
		case o.text != "":
			params.Text = []byte(o.text)
		case o.name != "":
			data, err := e.readStdin()
			if err != nil {
				return err
			}
			params.Files = []bundle.Source{memorySource(o.name, data)}
		case !e.stdinTTY:
			data, err := e.readStdin()
			if err != nil {
				return err
			}
			params.Text = data
		default:
			e.note("Type or paste the secret, then press Ctrl-D on an empty line.\n")
			data, err := e.readStdin()
			if err != nil {
				return err
			}
			// The Enter before Ctrl-D is not part of the secret.
			params.Text = bytes.TrimSuffix(bytes.TrimSuffix(data, []byte("\n")), []byte("\r"))
		}
	}
	if len(params.Files) == 0 && len(params.Text) == 0 {
		return share.ErrNothingToShare
	}

	password, err := passwordSource{flag: o.password, file: o.passwordFile}.resolve(e, false, true)
	if err != nil {
		return err
	}
	params.Password = password

	bar := e.progress("Encrypting and uploading…")
	params.Progress = bar.update
	result, err := share.Share(ctx, e.client(e.server), params)
	bar.finish()
	if err != nil {
		return err
	}
	return e.printShared(result, o)
}

func (e *env) readStdin() ([]byte, error) {
	data, err := io.ReadAll(e.stdin)
	if err != nil {
		return nil, fmt.Errorf("read stdin: %w", err)
	}
	return data, nil
}

func memorySource(name string, data []byte) bundle.Source {
	return bundle.Source{Name: name, Type: typeFor(name), Size: int64(len(data)), Reader: bytes.NewReader(data)}
}

func fileSource(path string) (bundle.Source, *os.File, error) {
	info, err := os.Stat(path)
	if err != nil {
		return bundle.Source{}, nil, err
	}
	if info.IsDir() {
		return bundle.Source{}, nil, fmt.Errorf("%s is a directory; pack it first: tar cz %s | secretli share --name %s.tgz", path, path, filepath.Base(path))
	}
	f, err := os.Open(path) //nolint:gosec // the path is what the person asked to share
	if err != nil {
		return bundle.Source{}, nil, err
	}
	name := filepath.Base(path)
	return bundle.Source{Name: name, Type: typeFor(name), Size: info.Size(), Reader: f}, f, nil
}

func typeFor(name string) string {
	t := mime.TypeByExtension(filepath.Ext(name))
	if t == "" {
		return "application/octet-stream"
	}
	if semi := strings.Index(t, ";"); semi >= 0 {
		t = strings.TrimSpace(t[:semi])
	}
	return t
}

func (e *env) printShared(r *share.Result, o shareOptions) error {
	link := r.Link.Recipient().String()
	if o.qr {
		if err := printQR(e.stderr, link); err != nil {
			e.note("Couldn't draw the QR code: %v\n", err)
		}
	}
	if o.copyLink {
		if err := copyToClipboard(link); err != nil {
			e.note("Couldn't copy to the clipboard: %v\n", err)
		} else {
			e.note("Copied the link to the clipboard.\n")
		}
	}
	if e.json {
		opens := "once"
		if r.Reusable {
			opens = "until_expiry"
		}
		out := map[string]any{
			"link":       link,
			"owner_link": r.Link.String(),
			"expires_at": r.ExpiresAt.UTC(),
			"opens":      opens,
			"password":   r.PasswordProtected,
			"kind":       r.Kind,
			"size":       r.Size,
		}
		if r.Kind == share.KindFiles {
			out["files"] = r.Names
		}
		return e.emitJSON(out)
	}

	opens := "Opens once"
	if r.Reusable {
		opens = "Opens until it expires"
	}
	summary := fmt.Sprintf("%s, expires %s.", opens, formatMoment(r.ExpiresAt, time.Now()))
	const ownerNote = "Owner link, keep it to yourself: it can delete the secret and shows whether it was opened."
	if e.quiet || !e.stdoutTTY {
		_, _ = fmt.Fprintln(e.stdout, link)
		if !e.quiet {
			_, _ = fmt.Fprintf(e.stderr, "%s\n%s\n%s\n", summary, ownerNote, r.Link.String())
		}
		return nil
	}
	_, _ = fmt.Fprintf(e.stdout, "Your link is ready. %s\n\n  %s\n\n%s\n\n  %s\n", summary, link, ownerNote, r.Link.String())
	return nil
}
