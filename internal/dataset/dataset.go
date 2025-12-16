package dataset

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"

	"github.com/NBISweden/sda-bpctl/cmd"
	"github.com/NBISweden/sda-bpctl/helpers"
	"github.com/NBISweden/sda-bpctl/internal/client"
	"github.com/NBISweden/sda-bpctl/internal/config"
	"github.com/NBISweden/sda-bpctl/internal/models"
	"github.com/spf13/cobra"
)

var dryRun bool
var configPath string
var dataDirectory string

var datasetCmd = &cobra.Command{
	Use:   "dataset [flags]",
	Short: "Trigger dataset creation",
	Long:  "Trigger dataset creation",
	Args: func(cmd *cobra.Command, args []string) error {
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.NewConfig(configPath)
		if err != nil {
			return err
		}
		datasetFolder := cfg.DatasetFolder
		datasetID := cfg.DatasetID
		userID := cfg.UserID

		api, err := client.New(cfg)
		if err != nil {
			return err
		}

		resp, err := api.GetUsersFilesWithPrefix()
		if err != nil {
			return err
		}

		var files []models.FileInfo
		if err := json.Unmarshal(resp, &files); err != nil {
			return err
		}

		if !dryRun {
			err := createStableIDsFile(datasetFolder, files)
			if err != nil {
				return err
			}
		}

		fileIDsList, err := getFileIDsFromFile(datasetFolder)
		if err != nil {
			return err
		}

		slog.Info("nr of files included in dataset", "nr_files", (len(fileIDsList)))
		if dryRun {
			slog.Info("dry run enabled, no dataset will be created")
			return nil
		}

		err = createDataset(api, datasetID, userID, fileIDsList)
		if err != nil {
			return err
		}

		return nil
	},
}

func init() {
	cmd.AddCommand(datasetCmd)
	datasetCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Toggles dry-run mode. Dry run will not run any state changing API calls")
	datasetCmd.Flags().StringVar(&configPath, "config", "config.yaml", "Path to configuration file")
	datasetCmd.Flags().StringVar(&dataDirectory, "data-directory", "data", "Path to directory to write / read intermediate files for stableIDs and fileIDs")
}

type Payload struct {
	AccessionIDs []string `json:"accession_ids"`
	DatasetID    string   `json:"dataset_id"`
	User         string   `json:"user"`
}

type UserFiles struct {
	AccessionID string `json:"accessionID"`
	InboxPath   string `json:"inboxPath"`
}

func Run(api *client.Client, datasetFolder string, datasetID string, userID string, fileIDsList []string) error {
	err := createDataset(api, datasetID, userID, fileIDsList)
	if err != nil {
		return err
	}
	return nil
}

func getFileIDsFromFile(datasetFolder string) ([]string, error) {
	var fileIDsList []string
	filePath := helpers.GetFileIDsPath(dataDirectory, datasetFolder)
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close() //nolint:errcheck

	slog.Info("reading", "filePath", filePath)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fileIDsList = append(fileIDsList, scanner.Text())
	}
	return fileIDsList, nil
}

func createDataset(api *client.Client, datasetID string, userID string, fileIDsList []string) error {
	slog.Info("starting dataset")

	if len(fileIDsList) > 100 {
		err := sendInChunks(fileIDsList, api, datasetID, userID)
		if err != nil {
			return err
		}
	}

	if len(fileIDsList) <= 100 {
		payload := Payload{
			AccessionIDs: fileIDsList,
			DatasetID:    datasetID,
			User:         userID,
		}
		jsonData, err := json.Marshal(payload)
		if err != nil {
			return err
		}

		_, err = api.PostDatasetCreate(jsonData)
		if err != nil {
			if errors.Is(err, io.ErrUnexpectedEOF) {
			} else {
				return err
			}
		}
	}

	slog.Info("creation of dataset completed!")
	return nil
}

func sendInChunks(fileIDsList []string, api *client.Client, datasetID string, userID string) error {
	slog.Info("more than 100 entries, sending in chunks of 100")
	chunks := slices.Chunk(fileIDsList, 100)
	allChunks := slices.Collect(chunks)
	for _, chunk := range allChunks {
		payload := Payload{
			AccessionIDs: chunk,
			DatasetID:    datasetID,
			User:         userID,
		}

		jsonData, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		_, err = api.PostDatasetCreate(jsonData)
		if err != nil {
			if errors.Is(err, io.ErrUnexpectedEOF) {
				continue
			}
			return err
		}
	}

	return nil
}

func createStableIDsFile(datasetFolder string, files []models.FileInfo) error {
	filePath := helpers.GetStableIDsPath(dataDirectory, datasetFolder)
	if _, err := os.Stat(filePath); err == nil {
		return err
	} else if !os.IsNotExist(err) {
		return err
	}

	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close() //nolint:errcheck

	for _, f := range files {
		fmt.Fprintf(file, "%s %s\n", f.AccessionID, f.InboxPath) //nolint:errcheck
	}

	slog.Info("created file with stable ids", "filePath", filePath)
	return nil
}
