package helpers

import (
	"bytes"
	"embed"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"text/template"

	"github.com/NBISweden/sda-bpctl/cmd"
	"github.com/NBISweden/sda-bpctl/internal/config"
	"github.com/NBISweden/sda-bpctl/internal/models"
	"github.com/spf13/cobra"
)

//go:embed templates/*.yaml
var templateFS embed.FS
var configPath string
var output string
var withXML bool

type TemplateData struct {
	WithDB                       bool
	WithXML                      bool
	JobName                      string
	JobReleaseLabel              string
	JobArgs                      string
	UserID                       string
	DatasetID                    string
	DatasetFolder                string
	SslCaCert                    string
	ClientAPIHost                string
	ClientAccessToken            string
	MailUploaderName             string
	MailUploaderOrganizationName string
	MailUploader                 string
	MailSecretName               string
	MailSMTPHost                 string
	MailSMTPPort                 string
	CertSecretName               string
	StorageSecretName            string
	DataDirectory                string
	C4GHSecretName               string
}

var renderCmd = &cobra.Command{
	Use:   "render [flags]",
	Short: "Render job manifest",
	Long:  "Render job manifest",
	Args: func(cmd *cobra.Command, args []string) error {
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.NewConfig(configPath)
		if err != nil {
			return err
		}

		templateData, err := createTemplateData(cfg)
		if err != nil {
			return err
		}

		jobManifest, err := renderTemplate(templateData)
		if err != nil {
			return err
		}

		err = writeManifest(jobManifest, output)
		if err != nil {
			return err
		}

		return nil
	},
}

func init() {
	cmd.AddCommand(renderCmd)
	renderCmd.Flags().StringVarP(&configPath, "config", "c", "config.yaml", "Path to configuration file")
	renderCmd.Flags().StringVarP(&output, "output", "o", "job.yaml", "Path to write the rendered file to")
	renderCmd.Flags().BoolVarP(&withXML, "xml", "x", false, "Render manifest with xml volumes included")
}

func createTemplateData(cfg *config.Config) (TemplateData, error) {
	templateData := &TemplateData{
		WithXML:                      withXML,
		JobName:                      strings.ToLower(strings.ReplaceAll(cfg.DatasetFolder, "_", "-")),
		JobReleaseLabel:              "sda",
		JobArgs:                      "[\"job\"]",
		UserID:                       cfg.UserID,
		DatasetID:                    cfg.DatasetID,
		DatasetFolder:                cfg.DatasetFolder,
		SslCaCert:                    "/.secrets/tls/ca.crt",
		ClientAPIHost:                "https://sda-sda-svc-api:8080",
		ClientAccessToken:            cfg.ClientAccessToken,
		MailUploaderName:             cfg.MailUploaderName,
		MailUploaderOrganizationName: cfg.MailUploaderOrganizationName,
		MailUploader:                 cfg.MailUploader,
		MailSecretName:               cfg.MailSecretName,
		MailSMTPHost:                 cfg.MailSMTPHost,
		MailSMTPPort:                 strconv.Itoa(cfg.MailSMTPPort),
		CertSecretName:               cfg.CertSecretName,
		StorageSecretName:            cfg.StorageSecretName,
		DataDirectory:                cfg.JobDataDirectory,
		C4GHSecretName:               "c4gh",
	}
	return *templateData, nil
}

func writeManifest(jobManifest string, output string) error {
	file, err := os.Create(output)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteString(jobManifest)
	if err != nil {
		return err
	}
	slog.Info("writing", "output", output)
	return nil
}

func renderTemplate(data TemplateData) (string, error) {
	tmpl, err := template.ParseFS(templateFS, "templates/job.template.yaml")
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func GetFileIDsPath(dataDirectory string, datasetFolder string) string {
	return fmt.Sprintf("%s/%s-fileIDs.txt", dataDirectory, datasetFolder)
}

func GetStableIDsPath(dataDirectory string, datasetFolder string) string {
	return fmt.Sprintf("%s/%s-stableIDs.txt", dataDirectory, datasetFolder)
}

// FilterFiles returns the files that belong to the dataset and have been uploaded.
func FilterFiles(files []models.FileInfo, datasetFolder string) []string {
	var filteredFiles []string
	for _, f := range FilterDatasetFiles(files, datasetFolder) {
		if f.Status != "uploaded" {
			continue
		}
		filteredFiles = append(filteredFiles, f.InboxPath)
	}
	return filteredFiles
}

// FilterDatasetFiles returns the files that belong to the dataset regardless
// of their status, i.e. files in datasetFolder excluding PRIVATE and
// LANDING_PAGE files.
func FilterDatasetFiles(files []models.FileInfo, datasetFolder string) []models.FileInfo {
	var filteredFiles []models.FileInfo
	for _, f := range files {
		if !strings.Contains(f.InboxPath, datasetFolder) {
			continue
		}
		if strings.Contains(f.InboxPath, "PRIVATE") || strings.Contains(f.InboxPath, "LANDING_PAGE") {
			continue
		}
		filteredFiles = append(filteredFiles, f)
	}
	return filteredFiles
}
