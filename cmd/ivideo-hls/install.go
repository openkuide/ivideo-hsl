package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/chamrong/ivideo-hls/internal/deps"
)

func newInstallDepsCommand() *cobra.Command {
	var force, skipChecksum bool
	cmd := &cobra.Command{
		Use:   "install-deps",
		Short: "Download and install ffmpeg + ffprobe into the user cache",
		Long: "Downloads pinned static builds of ffmpeg and ffprobe from their canonical upstream mirrors " +
			"(evermeet.cx on macOS, johnvansickle.com on Linux), verifies them against SHA-256 checksums " +
			"committed in this repo, and installs them to $XDG_CACHE_HOME/ivideo-hls/bin/.\n\n" +
			"Nothing is installed system-wide and no sudo is required. To remove, delete the cache directory.",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := deps.InstallOptions{
				Force:        force,
				SkipChecksum: skipChecksum,
				Progress: func(msg string) {
					fmt.Println("  " + msg)
				},
			}
			dir, err := deps.CacheBinDir()
			if err != nil {
				return err
			}
			fmt.Printf("Installing ffmpeg + ffprobe → %s\n", dir)
			if err := deps.Install(cmd.Context(), opts); err != nil {
				return err
			}
			fmt.Println(styleOk.Render("✔ dependencies ready"))
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "reinstall even if the binary already exists")
	cmd.Flags().BoolVar(&skipChecksum, "skip-checksum", false, "bypass SHA-256 verification (dev only)")
	return cmd
}
