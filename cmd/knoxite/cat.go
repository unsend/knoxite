/*
 * knoxite
 *     Copyright (c) 2016-2020, Christian Muehlhaeuser <muesli@gmail.com>
 *
 *   For license see LICENSE
 */

package main

import (
	"fmt"
	"os"

	"github.com/knoxite/knoxite"

	"github.com/spf13/cobra"
)

var (
	catCmd = &cobra.Command{
		Use:   "cat <snapshot> <file>",
		Short: "print file",
		Long:  `The cat command prints a file on the standard output`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return fmt.Errorf("cat needs a snapshot ID and filename")
			}
			return executeCat(args[0], args[1])
		},
	}
)

func init() {
	RootCmd.AddCommand(catCmd)
}

func executeCat(snapshotID string, file string) error {
	logger.Info("Opening repository")
	repository, err := openRepository(globalOpts.Repo, globalOpts.Password)
	if err != nil {
		return err
	}
	logger.Info("Opened repository")

	logger.Infof("Finding snapshot %s", snapshotID)
	_, snapshot, err := repository.FindSnapshot(snapshotID)
	if err != nil {
		return err
	}
	logger.Infof("Found snapshot %s", snapshot.Description)

	logger.Infof("Reading snapshot %s", snapshotID)
	if archive, ok := snapshot.Archives[file]; ok {
		logger.Infof("Found and read archive from location %s", archive.Path)

		logger.Info("Decoding archive data")
		b, _, err := knoxite.DecodeArchiveData(repository, *archive)
		if err != nil {
			return err
		}
		logger.Info("Decoded archive data")

		logger.Debug("Output file content")
		_, err = os.Stdout.Write(b)
		return err
	}

	return fmt.Errorf("%s: No such file or directory", file)
}
