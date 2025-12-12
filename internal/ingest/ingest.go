package ingest

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/NBISweden/submitter/cmd"
	"github.com/NBISweden/submitter/internal/client"
	"github.com/NBISweden/submitter/internal/config"
	"github.com/NBISweden/submitter/internal/database"
	"github.com/NBISweden/submitter/internal/models"
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
		files, err := api.GetUsersFiles()
		if err != nil {
			return err
		}
		_, err = ingestFiles(api, cfg.DatasetFolder, cfg.UserID, files)
		if err != nil {
			return err
		}

		return nil
	},
}

func init() {
	cmd.AddCommand(ingestCmd)
	ingestCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Toggles dry-run mode. Dry run will not run any state changing API calls")
	ingestCmd.Flags().StringVar(&configPath, "config", "config.yaml", "Path to configuration file")
}

func Run(api client.APIClient, db database.PostgresDb, datasetFolder string, userID string, expectedFiles int) (int, error) {
	files, err := db.GetUserFiles(userID, datasetFolder, true)
	if err != nil {
		return 0, err
	}

	filteredFiles := filterFiles(files, datasetFolder)
	if expectedFiles != len(filteredFiles) {
		return 0, fmt.Errorf("expected nr of files does not match files from db, got %d expected %d", len(files), expectedFiles)
	}
	return ingestFiles(api, datasetFolder, userID, files)
}

func filterFiles(files []models.FileInfo, datasetFolder string) []string {
	var filteredFiles []string
	for _, f := range files {
		if f.Status != "uploaded" {
			continue
		}
		if !strings.Contains(f.InboxPath, datasetFolder) {
			continue
		}
		if strings.Contains(f.InboxPath, "PRIVATE") || strings.Contains(f.InboxPath, "LANDING PAGE") {
			continue
		}
		filteredFiles = append(filteredFiles, f.InboxPath)
	}
	return filteredFiles
}

func ingestFiles(api client.APIClient, datasetFolder string, userID string, files []models.FileInfo) (int, error) {
	slog.Info("starting ingest")
	fileList := filterFiles(files, datasetFolder)
	filesCount := len(fileList)
	okResponds := len(fileList)

	slog.Info("number of files to ingest", "filesCount", filesCount)
	if dryRun {
		slog.Info("dry-run enabled. No files will be ingested")
		return filesCount, nil
	}

	for _, path := range fileList {
		payload := map[string]string{
			"filepath": path,
			"user":     userID,
		}
		data, _ := json.Marshal(payload)

		_, err := api.PostFileIngest(data)
		if err != nil {
			okResponds -= 1
			slog.Warn("file not ingested", "filepath", path, "err", err)
		}
	}

	slog.Info(fmt.Sprintf("ingested %d/%d successful responses", okResponds, filesCount))
	return okResponds, nil
}
