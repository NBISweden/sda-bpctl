package status

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/NBISweden/sda-bpctl/cmd"
	"github.com/NBISweden/sda-bpctl/internal/client"
	"github.com/NBISweden/sda-bpctl/internal/config"
	"github.com/NBISweden/sda-bpctl/internal/models"
	"github.com/spf13/cobra"
)

var configPath string

var statusCmd = &cobra.Command{
	Use:   "status [flags]",
	Short: "Report file status counts",
	Long:  "Reports how many files under the configured dataset folder are in each status, e.g. uploaded, verified, ready",
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

		counts, err := Run(api)
		if err != nil {
			return err
		}

		fmt.Print(FormatReport(counts))
		return nil
	},
}

func init() {
	cmd.AddCommand(statusCmd)
	statusCmd.Flags().StringVarP(&configPath, "config", "c", "config.yaml", "Path to configuration file")
}

func Run(api client.APIClient) (map[string]int, error) {
	slog.Info("fetching file status report")
	files, err := api.GetUsersFilesWithPrefix()
	if err != nil {
		return nil, err
	}

	return countByStatus(files), nil
}

func countByStatus(files []models.FileInfo) map[string]int {
	counts := make(map[string]int)
	for _, f := range files {
		if strings.Contains(f.InboxPath, "PRIVATE") || strings.Contains(f.InboxPath, "LANDING") {
			continue
		}
		counts[f.Status]++
	}
	return counts
}

// FormatReport renders the counts as lines sorted by count descending, then
// status name ascending, e.g.:
//
//	- verified: 2012
//	- uploaded: 124
//	- submitted: 2
func FormatReport(counts map[string]int) string {
	statuses := make([]string, 0, len(counts))
	for status := range counts {
		statuses = append(statuses, status)
	}

	sort.Slice(statuses, func(i, j int) bool {
		if counts[statuses[i]] != counts[statuses[j]] {
			return counts[statuses[i]] > counts[statuses[j]]
		}
		return statuses[i] < statuses[j]
	})

	var sb strings.Builder
	for _, status := range statuses {
		fmt.Fprintf(&sb, "- %s: %d\n", status, counts[status])
	}
	return sb.String()
}
