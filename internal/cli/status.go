package cli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/pscheid92/secretli/internal/share"
)

func newStatusCmd(e *env) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status [link]",
		Short: "Say what a link points to, without opening it",
		Long: `Describes a secret from its link alone: text or files, one-time or reusable,
password or not, when it was sent and when it expires. Nothing is opened, so
a one-time secret stays unopened.

For a secret that is gone it tells what happened: opened (and when), expired,
or deleted. The exit code is 4 then, so scripts can tell.`,
		Example: `  secretli status 'https://secretli.app/s#…!…'
  secretli status "$LINK" --json | jq -r .state`,
		Args: cobra.MaximumNArgs(1),
		RunE: run(func(cmd *cobra.Command, args []string) error {
			return e.status(cmd.Context(), args)
		}),
	}
	return cmd
}

// exitWith ends a command with a code and no message; the result was printed.
type exitWith struct{ code int }

func (x *exitWith) Error() string { return "" }

func (e *env) status(ctx context.Context, args []string) error {
	link, err := e.linkArg(args)
	if err != nil {
		return err
	}
	now := time.Now()
	info, err := share.Inspect(ctx, e.client(link.Origin), link)
	var gone *share.GoneError
	if errors.As(err, &gone) {
		if e.json {
			out := map[string]any{
				"state":           "gone",
				"outcome":         gone.Gone.Outcome,
				"one_time":        gone.Gone.BurnAfterRead,
				"ended_at":        gone.Gone.EndedAt.UTC(),
				"opened_by_owner": gone.Gone.OpenedByOwner,
			}
			if gone.Gone.FirstOpenedAt != nil {
				out["first_opened_at"] = gone.Gone.FirstOpenedAt.UTC()
			}
			if err := e.emitJSON(out); err != nil {
				return err
			}
		} else {
			_, _ = fmt.Fprintln(e.stdout, goneSentence(gone.Gone, link.IsOwner()))
		}
		return &exitWith{code: ExitGone}
	}
	if err != nil {
		return err
	}

	if e.json {
		out := map[string]any{
			"state":          "live",
			"kind":           info.Kind,
			"name":           info.BundleName,
			"password":       info.PasswordProtected,
			"reusable":       info.Reusable,
			"encrypted_size": info.EncryptedSize,
			"expires_at":     info.ExpiresAt.UTC(),
			"created_at":     info.CreatedAt.UTC(),
		}
		if info.OpenedAt != nil {
			out["opened_at"] = info.OpenedAt.UTC()
		}
		return e.emitJSON(out)
	}

	what := "A one-time "
	if info.Reusable {
		what = "A reusable "
	}
	if info.Kind == share.KindText {
		what += "text secret"
	} else {
		what += "set of files (" + info.BundleName + ")"
	}
	if info.PasswordProtected {
		what += " with a password"
	}
	_, _ = fmt.Fprintf(e.stdout, "%s.\nSent %s, expires %s.\n", what, formatMoment(info.CreatedAt, now), formatMoment(info.ExpiresAt, now))
	switch {
	case info.OpenedAt != nil:
		_, _ = fmt.Fprintf(e.stdout, "First opened %s.\n", formatMoment(*info.OpenedAt, now))
	default:
		_, _ = fmt.Fprintln(e.stdout, "Not opened yet.")
	}
	return nil
}
