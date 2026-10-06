package job

import (
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/NBISweden/sda-bpctl/cmd"
	"github.com/NBISweden/sda-bpctl/helpers"
	"github.com/NBISweden/sda-bpctl/internal/accession"
	"github.com/NBISweden/sda-bpctl/internal/client"
	"github.com/NBISweden/sda-bpctl/internal/config"
	"github.com/NBISweden/sda-bpctl/internal/dataset"
	"github.com/NBISweden/sda-bpctl/internal/ingest"
	"github.com/NBISweden/sda-bpctl/internal/landingpage"
	"github.com/NBISweden/sda-bpctl/internal/mail"
	"github.com/spf13/cobra"
)

var configPath string

var jobCmd = &cobra.Command{
	Use:   "job",
	Short: "Runs all dataset submission steps in order",
	Long:  `Runs all dataset submission steps in order (ingestion -> accession -> dataset) takes a integer value representing the expected number of files to be included in the finalized dataset as argument. When a dataset is completed it ends with sending mail notifications and moving landing pages`,

	RunE: func(cmd *cobra.Command, args []string) error {
		err := runJob()
		if err != nil {
			return err
		}
		return nil
	},
}

func init() {
	cmd.AddCommand(jobCmd)
	jobCmd.Flags().StringVarP(&configPath, "config", "c", "config.yaml", "Path to configuration file")
}

func runJob() error {
	cfg, err := config.NewConfig(configPath)
	if err != nil {
		return err
	}

	pollRate := time.Minute * time.Duration(cfg.PollRate)
	timeout := time.Minute * time.Duration(cfg.Timeout)
	datasetFolder := cfg.DatasetFolder
	datasetID := cfg.DatasetID
	userID := cfg.UserID
	dataDirectory := cfg.JobDataDirectory

	slog.Info("dispatching job", "dataset_folder", datasetFolder, "dataset_id", datasetID, "userID", userID)

	api, err := client.New(cfg)
	if err != nil {
		return err
	}

	_, err = ingest.Run(api, datasetFolder, userID)
	if err != nil {
		return err
	}

	// The job may be resumed from a previous run where some files were already
	// ingested, accessioned or mapped, so the number of files ingested in this
	// run cannot be used as target. Instead the target is all files in the
	// dataset folder, regardless of their status.
	nrDatasetFiles, nrMappedFiles, err := countDatasetFiles(api, datasetFolder, datasetID)
	if err != nil {
		return err
	}
	nrTotalFiles := nrDatasetFiles + nrMappedFiles
	slog.Info("files in dataset", "nr_files", nrTotalFiles, "nr_not_mapped", nrDatasetFiles, "nr_already_mapped", nrMappedFiles)

	// Files accessioned in a previous run may be "ready" already.
	err = waitForDatasetFiles(api, datasetFolder, nrDatasetFiles, []string{"verified", "ready"}, pollRate, timeout)
	if err != nil {
		return err
	}

	accession.DataDirectory = dataDirectory
	_, err = accession.Run(api, datasetFolder, userID)
	if err != nil {
		return err
	}

	// Accession is processed asynchronously, so wait until every file has an
	// accession ID before creating the dataset, otherwise only a subset of the
	// files would be mapped to it.
	err = waitForDatasetFiles(api, datasetFolder, nrDatasetFiles, []string{"ready"}, pollRate, timeout)
	if err != nil {
		return err
	}

	// When resuming a run where every file was already mapped there is nothing
	// left to add, and dataset/create would be called with an empty list.
	if nrDatasetFiles > 0 {
		dataset.DataDirectory = dataDirectory
		err = dataset.Run(api, datasetFolder, datasetID, userID)
		if err != nil {
			return err
		}
	} else {
		slog.Info("all files already mapped to dataset, skipping dataset creation", "dataset_id", datasetID)
	}

	// Mapping files to the dataset is also processed asynchronously, so wait
	// until all files are mapped before the landing page and mail steps,
	// otherwise notifications would be sent for a partial dataset.
	err = waitForDatasetMapping(api, datasetID, nrTotalFiles, pollRate, timeout)
	if err != nil {
		return err
	}

	err = landingpage.Run(cfg)
	if err != nil {
		slog.Warn("could not complete landingpage", "err", err)
	}

	mail.DataDirectory = dataDirectory
	err = mail.Run(cfg)
	if err != nil {
		slog.Warn("could not complete mail notifications", "err", err)
	}

	slog.Info("dataset submission completed!")
	return nil
}

// countDatasetFiles returns the number of dataset files not yet mapped to the
// dataset and the number already mapped to it. The two have to be counted
// separately because GET /users/<user>/files only lists files that are not part
// of a dataset, so on a resumed run the listing alone undercounts the dataset.
//
// The listing is fetched before the dataset, so a file mapped in between by a
// previous run is counted twice rather than missed: the job then times out
// instead of sending notifications for a partial dataset.
func countDatasetFiles(api client.APIClient, datasetFolder string, datasetID string) (notMapped int, mapped int, err error) {
	allFiles, err := api.GetUsersFilesWithPrefix()
	if err != nil {
		return 0, 0, err
	}
	notMapped = len(helpers.FilterDatasetFiles(allFiles, datasetFolder))

	ds, err := api.GetDataset(datasetID)
	if err != nil {
		return 0, 0, err
	}
	if ds != nil {
		mapped = ds.NumberOfFiles
	}

	if notMapped+mapped == 0 {
		return 0, 0, fmt.Errorf("no files found in dataset folder %s and no files mapped to dataset %s", datasetFolder, datasetID)
	}
	return notMapped, mapped, nil
}

// waitForDatasetFiles polls until at least target files in the dataset folder
// have one of the given statuses. It fails early if any file ends up in
// "error" status since the dataset can then never be completed.
func waitForDatasetFiles(api client.APIClient, datasetFolder string, target int, statuses []string, interval time.Duration, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		allFiles, err := api.GetUsersFilesWithPrefix()
		if err != nil {
			return err
		}

		current := 0
		for _, f := range helpers.FilterDatasetFiles(allFiles, datasetFolder) {
			if f.Status == "error" {
				return fmt.Errorf("file %s has status error", f.InboxPath)
			}
			if slices.Contains(statuses, f.Status) {
				current++
			}
		}

		if current >= target {
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timeout reached, only got %d/%d files with status %v", current, target, statuses)
		}
		slog.Info(fmt.Sprintf("found %d/%d files with status %v - waiting: %s timeout: %s", current, target, statuses, interval, timeout))
		time.Sleep(interval)
	}
}

// waitForDatasetMapping polls until at least target files are mapped to the
// dataset.
func waitForDatasetMapping(api client.APIClient, datasetID string, target int, interval time.Duration, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		ds, err := api.GetDataset(datasetID)
		if err != nil {
			return err
		}

		current := 0
		if ds != nil {
			current = ds.NumberOfFiles
		}

		if current >= target {
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timeout reached, only %d/%d files mapped to dataset %s", current, target, datasetID)
		}
		slog.Info(fmt.Sprintf("found %d/%d files mapped to dataset %s - waiting: %s timeout: %s", current, target, datasetID, interval, timeout))
		time.Sleep(interval)
	}
}
