package cli

import (
	"context"
	"errors"

	"github.com/spf13/cobra"

	"github.com/pscheid92/secretli/internal/share"
)

func newDeleteCmd(e *env) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "delete <owner-link>",
		Short: "Delete a secret for everyone",
		Long: `Deletes a secret before it is opened or expires. Only the owner link, the one
with the part after "!", can do this. The link stops working right away.`,
		Example: `  secretli delete 'https://secretli.app/s#…!…'
  secretli delete "$OWNER_LINK" --yes`,
		Args: cobra.MaximumNArgs(1),
		RunE: run(func(cmd *cobra.Command, args []string) error {
			return e.delete(cmd.Context(), args, yes)
		}),
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "don't ask; needed when not at a terminal")
	return cmd
}

func (e *env) delete(ctx context.Context, args []string, yes bool) error {
	link, err := e.linkArg(args)
	if err != nil {
		return err
	}
	if !link.IsOwner() {
		return share.ErrNotOwner
	}
	if !yes {
		t, err := e.terminal()
		if err != nil {
			return errors.New("use --yes to delete without being asked")
		}
		ok, err := t.confirm("Delete it for everyone? The link stops working right away.")
		t.close()
		if err != nil {
			return err
		}
		if !ok {
			return &exitWith{code: ExitError}
		}
	}
	if err := share.Delete(ctx, e.client(link.Origin), link); err != nil {
		return describeGone(err, true)
	}
	if e.json {
		return e.emitJSON(map[string]any{"deleted": true})
	}
	e.say("Deleted. The link doesn't open anything any more.\n")
	return nil
}
