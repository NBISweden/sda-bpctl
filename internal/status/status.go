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
var statusFilter string
var sqlFormat bool

var statusCmd = &cobra.Command{
	Use:   "status [flags]",
	Short: "Report file status counts",
	Long:  "Reports how many files under the configured dataset folder are in each status, e.g. uploaded, verified, ready. Use --status to instead list the file IDs currently in a specific status, optionally formatted as a SQL IN clause with --sql.",
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

		files, err := Run(api)
		if err != nil {
			return err
		}

		if statusFilter != "" {
			ids := FileIDsForStatus(files, statusFilter)
			if sqlFormat {
				fmt.Println(FormatSQLInClause(ids))
				return nil
			}
			for _, id := range ids {
				fmt.Println(id)
			}
			return nil
		}

		fmt.Print(FormatReport(CountByStatus(files)))
		return nil
	},
}

func init() {
	cmd.AddCommand(statusCmd)
	statusCmd.Flags().StringVarP(&configPath, "config", "c", "config.yaml", "Path to configuration file")
	statusCmd.Flags().StringVarP(&statusFilter, "status", "s", "", "Only list the file IDs currently in this status, instead of printing the full report")
	statusCmd.Flags().BoolVar(&sqlFormat, "sql", false, "With --status, format the file IDs as a SQL IN clause, e.g. ('id1', 'id2') instead of one per line")
}

func Run(api client.APIClient) ([]models.FileInfo, error) {
	slog.Info("fetching file status report")
	files, err := api.GetUsersFilesWithPrefix()
	if err != nil {
		return nil, err
	}

	return filterFiles(files), nil
}

func filterFiles(files []models.FileInfo) []models.FileInfo {
	var filtered []models.FileInfo
	for _, f := range files {
		if strings.Contains(f.InboxPath, "PRIVATE") || strings.Contains(f.InboxPath, "LANDING") {
			continue
		}
		filtered = append(filtered, f)
	}
	return filtered
}

func CountByStatus(files []models.FileInfo) map[string]int {
	counts := make(map[string]int)
	for _, f := range files {
		counts[f.Status]++
	}
	return counts
}

// FileIDsForStatus returns the FileID of every file whose Status matches status.
func FileIDsForStatus(files []models.FileInfo, status string) []string {
	var ids []string
	for _, f := range files {
		if f.Status == status {
			ids = append(ids, f.FileID)
		}
	}
	return ids
}

// FormatSQLInClause renders ids in a parenthesized, comma seperated, singled quoted format.
// easier to use it directly in a psql query.
func FormatSQLInClause(ids []string) string {
	quoted := make([]string, len(ids))
	for i, id := range ids {
		quoted[i] = "'" + strings.ReplaceAll(id, "'", "''") + "'"
	}
	return "(" + strings.Join(quoted, ", ") + ")"
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
