package job

import (
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/NBISweden/sda-bpctl/cmd"
	"github.com/NBISweden/sda-bpctl/internal/accession"
	"github.com/NBISweden/sda-bpctl/internal/client"
	"github.com/NBISweden/sda-bpctl/internal/config"
	"github.com/NBISweden/sda-bpctl/internal/dataset"
	"github.com/NBISweden/sda-bpctl/internal/ingest"
	"github.com/spf13/cobra"
)

var configPath string
var expectedFiles int

var jobCmd = &cobra.Command{
	Use:   "job <expectedFiles>",
	Short: "Runs all dataset submission steps as a 'job'",
	Long: `Runs all dataset submission steps as a 'job' (ingestion, accession, dataset) takes a integer value representing the expected number of files to be included in the finalized dataset as argument
	`,
	Args: func(cmd *cobra.Command, args []string) error {
		var err error
		if len(args) == 0 {
			return fmt.Errorf("job must be supplied a number of expected files as argument")
		}

		if len(args) > 1 {
			return fmt.Errorf("job can only handle one argument")
		}

		expectedFiles, err = strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("could not interpert expected number of files %w", err)
		}
		return nil
	},

	RunE: func(cmd *cobra.Command, args []string) error {
		err := runJob(expectedFiles)
		if err != nil {
			return err
		}
		return nil
	},
}

func init() {
	cmd.AddCommand(jobCmd)
	jobCmd.Flags().StringVar(&configPath, "config", "config.yaml", "Path to configuration file")
}

func runJob(expectedFiles int) error {
	cfg, err := config.NewConfig(configPath)
	if err != nil {
		return err
	}

	pollRate := time.Minute * time.Duration(cfg.PollRate)
	timeout := time.Minute * time.Duration(cfg.Timeout)
	datasetFolder := cfg.DatasetFolder
	datasetID := cfg.DatasetID
	userID := cfg.UserID

	slog.Info("dispatching job", "dataset_folder", datasetFolder, "dataset_id", datasetID, "userID", userID, "expected_files", expectedFiles)

	api, err := client.New(cfg)
	if err != nil {
		return err
	}

	filesCount, err := ingest.Run(api, datasetFolder, userID, expectedFiles)
	if err != nil {
		return err
	}

	if filesCount != expectedFiles {
		return fmt.Errorf("ingest did not return the expected number of files, got %d expected %d", filesCount, expectedFiles)
	}

	_, err = api.WaitForAccession(filesCount, pollRate, timeout)
	if err != nil {
		return err
	}

	accessionIDs, err := accession.Run(api, datasetFolder, userID)
	if err != nil {
		return err
	}

	nrAccessionIDs := len(accessionIDs)
	if nrAccessionIDs != expectedFiles {
		return fmt.Errorf("accession did not return the expected number of files, got %d expected %d", nrAccessionIDs, expectedFiles)
	}

	// We give some time for the SDA backend to process our accession ids. During test-runs it's been fine with 10 minutes
	waitTime := 10 * time.Minute
	slog.Info("waiting before sending dataset creation request", "delay", waitTime)
	time.Sleep(waitTime)

	err = dataset.Run(api, datasetFolder, datasetID, userID, accessionIDs)
	if err != nil {
		return err
	}

	slog.Info("dataset submission completed!")
	return nil
}
