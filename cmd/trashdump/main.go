package main

import (
	"context"
	"os"

	"github.com/idkmaybdeveloper/trashdump/internal/dump"
	"github.com/spf13/cobra"
)

func main() {
	var opts dump.Options

	cmd := &cobra.Command{
		Use:          "trashdump <image>",
		Short:        "Pull and dump OCI image rootfs to a directory",
		Example:      "trashdump -o ./rootfs alpine:edge",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return dump.Dump(cmd.Context(), args[0], opts)
		},
	}

	cmd.Flags().StringVarP(&opts.OutputDir, "output", "o", "", "output directory (default: derived from image ref)")
	cmd.Flags().StringVarP(&opts.Platform, "platform", "p", "linux/amd64", "target platform (os/arch[/variant])")
	cmd.Flags().StringVarP(&opts.Username, "username", "u", "", "registry username")
	cmd.Flags().StringVarP(&opts.Password, "password", "P", "", "registry password")
	cmd.Flags().BoolVar(&opts.Insecure, "insecure", false, "allow plain HTTP registries")

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		os.Exit(1)
	}
}
