package ingest

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/NBISweden/sda-bpctl/cmd"
	"github.com/NBISweden/sda-bpctl/internal/client"
	"github.com/NBISweden/sda-bpctl/internal/config"
	"github.com/NBISweden/sda-bpctl/internal/models"
	"github.com/spf13/cobra"
)

var dryRun bool
var configPath string

var ingestCmd = &cobra.Command{
	Use:   "ingest [flags]",
	Short: "Trigger ingestion",
	Long:  "Trigger ingestion",
	Args: func(cmd *cobra.Command, args []string) error {
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.NewConfig(configPath)
		if err != nil {
			return err
		}
		api, err := client.New(cfg)
		if err != nil {
			return err
		}
		_, err = Run(api, cfg.DatasetFolder, cfg.UserID)
		if err != nil {
			return err
		}

		return nil
	},
}

func init() {
	cmd.AddCommand(ingestCmd)
	ingestCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Toggles dry-run mode. Dry run will not run any state changing API calls")
	ingestCmd.Flags().StringVarP(&configPath, "config", "c", "config.yaml", "Path to configuration file")
}

func Run(api client.APIClient, datasetFolder string, userID string) (int, error) {
	files, err := api.GetFilesWithStatus("uploaded")
	if err != nil {
		return 0, err
	}

	return ingestFiles(api, userID, files)
}

func ingestFiles(api client.APIClient, userID string, files []models.FileInfo) (int, error) {
	slog.Info("starting ingest")
	filesCount := len(files)
	okResponses := len(files)

	slog.Info("number of files to ingest", "filesCount", filesCount)
	if dryRun {
		slog.Info("dry-run enabled. No files will be ingested")
		return filesCount, nil
	}

	for _, file := range files {
		payload := map[string]string{
			"filepath": file.InboxPath,
			"user":     userID,
		}
		data, _ := json.Marshal(payload)

		_, err := api.PostFileIngest(data)
		if err != nil {
			okResponses--
			slog.Warn("file not ingested", "filepath", file.InboxPath, "err", err)
		}
	}

	slog.Info(fmt.Sprintf("ingested %d/%d successful responses", okResponses, filesCount))
	return okResponses, nil
}
