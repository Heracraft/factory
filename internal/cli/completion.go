package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newCompletionCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "completion bash|zsh|fish",
		Short:     "Generate shell completion",
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs: []string{"bash", "zsh", "fish"},
		RunE: func(cmd *cobra.Command, args []string) error {
			root := cmd.Root()
			switch args[0] {
			case "bash":
				return root.GenBashCompletion(os.Stdout)
			case "zsh":
				return root.GenZshCompletion(os.Stdout)
			case "fish":
				return root.GenFishCompletion(os.Stdout, true)
			default:
				return fmt.Errorf("unknown shell %q", args[0])
			}
		},
	}
}
