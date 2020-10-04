/*
 * knoxite
 *     Copyright (c) 2016-2020, Christian Muehlhaeuser <muesli@gmail.com>
 *
 *   For license see LICENSE
 */

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	humanize "github.com/dustin/go-humanize"
	shutdown "github.com/klauspost/shutdown2"
	"github.com/muesli/goprogressbar"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/knoxite/knoxite"
	"github.com/knoxite/knoxite/cmd/knoxite/utils"
)

// Error declarations.
var (
	ErrRedundancyAmount = errors.New("failure tolerance can't be equal or higher as the number of storage backends")
)

// StoreOptions holds all the options that can be set for the 'store' command.
type StoreOptions struct {
	Description      string
	Compression      string
	Encryption       string
	FailureTolerance uint
	Excludes         []string
	Pedantic         bool
}

var (
	storeOpts = StoreOptions{}

	storeCmd = &cobra.Command{
		Use:   "store <volume> <dir/file> [...]",
		Short: "store files/directories",
		Long:  `The store command creates a snapshot of a file or directory`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("store needs to know which volume to create a snapshot in")
			}
			if len(args) < 2 {
				return fmt.Errorf("store needs to know which files and/or directories to work on")
			}

			configureStoreOpts(cmd, &storeOpts)
			return executeStore(args[0], args[1:], storeOpts)
		},
	}
)

// configureStoreOpts will compare the settings from the configuration file and
// the user set command line flags.
// Values set via the command line flags will overwrite settings stored in the
// configuration file.
func configureStoreOpts(cmd *cobra.Command, opts *StoreOptions) {
	if rep, ok := cfg.Repositories[globalOpts.Alias]; ok {
		if !cmd.Flags().Changed("compression") {
			opts.Compression = rep.Compression
		}
		if !cmd.Flags().Changed("encryption") {
			opts.Encryption = rep.Encryption
		}
		if !cmd.Flags().Changed("tolerance") {
			opts.FailureTolerance = rep.Tolerance
		}
		if !cmd.Flags().Changed("excludes") {
			opts.Excludes = rep.StoreExcludes
		}
		if !cmd.Flags().Changed("pedantic") {
			opts.Pedantic = rep.Pedantic
		}
	}
}

func initStoreFlags(f func() *pflag.FlagSet, opts *StoreOptions) {
	f().StringVarP(&opts.Description, "desc", "d", "", "a description or comment for this volume")
	f().StringVarP(&opts.Compression, "compression", "c", "", "compression algo to use: none (default), flate, gzip, lzma, zlib, zstd")
	f().StringVarP(&opts.Encryption, "encryption", "e", "", "encryption algo to use: aes (default), none")
	f().UintVarP(&opts.FailureTolerance, "tolerance", "t", 0, "failure tolerance against n backend failures")
	f().StringArrayVarP(&opts.Excludes, "excludes", "x", []string{}, "list of excludes")
	f().BoolVar(&opts.Pedantic, "pedantic", false, "exit on first error")
}

func init() {
	initStoreFlags(storeCmd.Flags, &storeOpts)
	RootCmd.AddCommand(storeCmd)
}

func store(repository *knoxite.Repository, chunkIndex *knoxite.ChunkIndex, snapshot *knoxite.Snapshot, targets []string, opts StoreOptions) error {
	// we want to be notified during the first phase of a shutdown
	logger.Info("Acquiring shutdown notifier")
	cancel := shutdown.First()

	logger.Info("Getting rooted path name corresponding to the current directory")
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	logger.Infof("Got rooted path: %s", wd)

	if len(repository.BackendManager().Backends)-int(opts.FailureTolerance) <= 0 {
		return ErrRedundancyAmount
	}

	logger.Info("Get compression type from options")
	compression, err := utils.CompressionTypeFromString(opts.Compression)
	if err != nil {
		return err
	}

	logger.Info("Get encryption type from options")
	encryption, err := utils.EncryptionTypeFromString(opts.Encryption)
	if err != nil {
		return err
	}

	so := knoxite.StoreOptions{
		CWD:         wd,
		Paths:       targets,
		Excludes:    opts.Excludes,
		Compress:    compression,
		Encrypt:     encryption,
		Pedantic:    opts.Pedantic,
		DataParts:   uint(len(repository.BackendManager().Backends) - int(opts.FailureTolerance)),
		ParityParts: opts.FailureTolerance,
	}

	startTime := time.Now()
	logger.Info("Adding snapshot and getting progress")
	progress := snapshot.Add(*repository, chunkIndex, so)

	logger.Debug("Initializing new goprogressbar for output")
	fileProgressBar := &goprogressbar.ProgressBar{Width: 40}
	overallProgressBar := &goprogressbar.ProgressBar{
		Text:  fmt.Sprintf("%d of %d total", 0, 0),
		Width: 60,
		PrependTextFunc: func(p *goprogressbar.ProgressBar) string {
			return fmt.Sprintf("%s/s",
				knoxite.SizeToString(uint64(float64(p.Current)/time.Since(startTime).Seconds())))
		},
	}

	pb := goprogressbar.MultiProgressBar{}
	pb.AddProgressBar(fileProgressBar)
	pb.AddProgressBar(overallProgressBar)
	lastPath := ""

	items := int64(1)
	errs := make(map[string]error)

	logger.Debug("Iterating over progress to print details")
	for p := range progress {
		select {
		case n := <-cancel:
			fmt.Println("Aborting...")
			close(n)
			return nil

		default:
			if p.Error != nil {
				if storeOpts.Pedantic {
					fmt.Println()
					return p.Error
				}
				errs[p.Path] = p.Error
				snapshot.Stats.Errors++
			}
			if p.Path != lastPath && lastPath != "" {
				items++
				fmt.Println()
			}
			fileProgressBar.Total = int64(p.CurrentItemStats.Size)
			fileProgressBar.Current = int64(p.CurrentItemStats.Transferred)
			fileProgressBar.PrependText = fmt.Sprintf("%s  %s/s",
				knoxite.SizeToString(uint64(fileProgressBar.Current)),
				knoxite.SizeToString(p.TransferSpeed()))

			overallProgressBar.Total = int64(p.TotalStatistics.Size)
			overallProgressBar.Current = int64(p.TotalStatistics.Transferred)
			overallProgressBar.Text = fmt.Sprintf("%s / %s (%s of %s)",
				knoxite.SizeToString(uint64(overallProgressBar.Current)),
				knoxite.SizeToString(uint64(overallProgressBar.Total)),
				humanize.Comma(items),
				humanize.Comma(int64(p.TotalStatistics.Files+p.TotalStatistics.Dirs+p.TotalStatistics.SymLinks)))

			if p.Path != lastPath {
				lastPath = p.Path
				fileProgressBar.Text = p.Path
			}

			pb.LazyPrint()
		}
	}

	fmt.Printf("\nSnapshot %s created: %s\n", snapshot.ID, snapshot.Stats.String())
	for file, err := range errs {
		fmt.Printf("'%s': failed to store: %v\n", file, err)
	}
	return nil
}

func executeStore(volumeID string, args []string, opts StoreOptions) error {
	targets := []string{}

	logger.Info("Collecting targets")
	for _, target := range args {
		if absTarget, err := filepath.Abs(target); err == nil {
			target = absTarget
		}
		targets = append(targets, target)
	}
	logger.Info("Collected targets")

	// we don't want these next calls to be interrupted
	logger.Info("Acquiring shutdown lock")
	lock := shutdown.Lock()
	if lock == nil {
		return nil
	}
	logger.Info("Acquired and locked shutdown lock")

	logger.Info("Opening repository")
	repository, err := openRepository(globalOpts.Repo, globalOpts.Password)
	if err != nil {
		return err
	}
	logger.Info("Opened repository")

	logger.Infof("Finding volume %s", volumeID)
	volume, err := repository.FindVolume(volumeID)
	if err != nil {
		return err
	}
	logger.Info("Found volume")

	logger.Infof("Creating new snapshot: %s", opts.Description)
	snapshot, err := knoxite.NewSnapshot(opts.Description)
	if err != nil {
		return err
	}
	logger.Info("Created snapshot")

	logger.Info("Opening chunk index")
	chunkIndex, err := knoxite.OpenChunkIndex(&repository)
	if err != nil {
		return err
	}
	logger.Info("Opened chunk index")

	// release the shutdown lock
	lock()
	logger.Info("Shutdown lock released")

	logger.Infof("Storing snapshot %s", snapshot.ID)
	err = store(&repository, &chunkIndex, snapshot, targets, opts)
	if err != nil {
		return err
	}
	logger.Info("Stored snapshot")

	// we don't want these next calls to be interrupted
	logger.Info("Acquiring another shutdown lock")
	lock = shutdown.Lock()
	if lock == nil {
		return nil
	}
	defer lock()
	defer logger.Info("Shutdown lock released")

	logger.Info("Saving snapshot")
	err = snapshot.Save(&repository)
	if err != nil {
		return err
	}
	logger.Info("Saved snapshot")

	logger.Infof("Adding snapshot to volume %s", volume.ID)
	err = volume.AddSnapshot(snapshot.ID)
	if err != nil {
		return err
	}
	logger.Info("Added snapshot to volume")

	logger.Info("Saving chunk index")
	err = chunkIndex.Save(&repository)
	if err != nil {
		return err
	}
	logger.Info("Saved chunk index")

	logger.Info("Saving repository")
	err = repository.Save()
	if err != nil {
		return err
	}
	logger.Info("Saved repository")
	return nil
}
