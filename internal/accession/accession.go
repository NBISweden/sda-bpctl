package accession

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"os"
	"strings"

	"github.com/NBISweden/sda-bpctl/cmd"
	"github.com/NBISweden/sda-bpctl/helpers"
	"github.com/NBISweden/sda-bpctl/internal/client"
	"github.com/NBISweden/sda-bpctl/internal/config"
	"github.com/NBISweden/sda-bpctl/internal/database"
	"github.com/NBISweden/sda-bpctl/internal/models"
	"github.com/spf13/cobra"
)

var dryRun bool
var configPath string
var dataDirectory string
var datasetFolder string

var accessionCmd = &cobra.Command{
	Use:   "accession [flags]",
	Short: "Trigger accession",
	Long:  "Trigger accession",
	Args: func(cmd *cobra.Command, args []string) error {
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.NewConfig(configPath)
		if err != nil {
			return err
		}

		datasetFolder := cfg.DatasetFolder
		userID := cfg.UserID

		api, err := client.New(cfg)
		if err != nil {
			return err
		}

		filePath := helpers.GetFileIDsPath(dataDirectory, datasetFolder)
		file, err := createFileIDFile(filePath, dryRun)
		if err != nil {
			slog.Error("error occoured when trying to create file", "filePath", filePath)
			return err
		}
		defer file.Close() //nolint:errcheck

		files, err := api.GetUsersFiles()
		if err != nil {
			return err
		}

		paths := getPathsForAccessionIDs(files)
		if dryRun {
			slog.Info("dry run enabled, no accession ids will be created")
			return nil
		}
		accessionIDs, err := postAccessionIDs(api, paths, userID)

		for _, accessionID := range accessionIDs {
			if _, err := file.WriteString(accessionID + "\n"); err != nil {
				return err
			}
		}

		if err != nil {
			return err
		}

		return nil
	},
}

func init() {
	cmd.AddCommand(accessionCmd)
	accessionCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Toggles dry-run mode. Dry run will not run any state changing API calls")
	accessionCmd.Flags().StringVar(&configPath, "config", "config.yaml", "Path to configuration file")
	accessionCmd.Flags().StringVar(&dataDirectory, "data-directory", "data", "Path to directory to write / read intermediate files for stableIDs and fileIDs")
}

func Run(api client.APIClient, db database.PostgresDb, datasetFolder string, userID string) ([]string, error) {
	slog.Info("starting accession")
	files, err := db.GetUserFiles(userID, datasetFolder, true)
	if err != nil {
		return nil, err
	}

	paths := getPathsForAccessionIDs(files)
	accessionIDs, err := postAccessionIDs(api, paths, userID)
	if err != nil {
		return nil, err
	}

	slog.Info("accession complete")
	return accessionIDs, nil
}

func getPathsForAccessionIDs(files []models.FileInfo) []string {
	var paths []string
	for _, f := range files {
		if f.Status == "verified" &&
			strings.Contains(f.InboxPath, datasetFolder) &&
			!strings.Contains(f.InboxPath, "PRIVATE") {
			paths = append(paths, f.InboxPath)
		}
	}
	slog.Info("files found for accession id creation", "files_found", len(paths))
	return paths
}

func postAccessionIDs(api client.APIClient, paths []string, userID string) ([]string, error) {
	var accessionIDs []string
	for _, filepath := range paths {
		accessionID, err := generateAccessionID()
		if err != nil {
			return accessionIDs, err
		}

		payload, err := json.Marshal(map[string]string{
			"accession_id": accessionID,
			"filepath":     filepath,
			"user":         userID,
		})
		if err != nil {
			return accessionIDs, err
		}

		_, err = api.PostFileAccession(payload)
		if err != nil {
			if errors.Is(err, io.ErrUnexpectedEOF) {
				continue
			}
			return accessionIDs, err
		}
		accessionIDs = append(accessionIDs, accessionID)
	}

	slog.Info("accession IDs assigned", "nr_files", len(paths))
	return accessionIDs, nil
}

func createFileIDFile(fileIDPath string, dryrun bool) (*os.File, error) {
	if dryrun {
		return nil, nil
	}

	if _, err := os.Stat(fileIDPath); err == nil {
		return nil, fmt.Errorf("file already exists")
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	file, err := os.Create(fileIDPath)
	if err != nil {
		return nil, err
	}

	return file, nil
}

func generateAccessionID() (string, error) {
	const chars = "abcdefghijklmnopqrstuvxyz23456789"
	const length = 6

	genPart := func() (string, error) {
		result := make([]byte, length)
		for i := range length {
			n, err := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
			if err != nil {
				return "", err
			}
			result[i] = chars[n.Int64()]
		}
		return string(result), nil
	}
	partOne, err := genPart()
	if err != nil {
		return "", err
	}

	partTwo, err := genPart()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("aa-File-%s-%s", partOne, partTwo), nil
}
