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
	"github.com/spf13/cobra"
)

//go:embed templates/*.yaml
var templateFS embed.FS
var configPath string
var output string

type TemplateData struct {
	JobName          string
	JobReleaseLabel  string
	JobArgs          string
	UserID           string
	DatasetID        string
	DatasetFolder    string
	SslCaCert        string
	ClientApiHost    string
	ClientAccesToken string
	MailUploaderName string
	MailUploader     string
	MailAddress      string
	MailPassword     string
	MailSmptHost     string
	MailSmptPort     string
	DbSecretName     string
	DbCaCert         string
	DbClientCert     string
	DbClientKey      string
	CertSecretName   string
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
}

func createTemplateData(cfg *config.Config) (TemplateData, error) {
	templateData := &TemplateData{
		JobName:          strings.ToLower(strings.ReplaceAll(cfg.DatasetFolder, "_", "-")),
		JobReleaseLabel:  "sda",
		JobArgs:          fmt.Sprintf("[\"job\", \"%d\"]", cfg.ExpectedNrFiles),
		UserID:           cfg.UserID,
		DatasetID:        cfg.DatasetID,
		DatasetFolder:    cfg.DatasetFolder,
		SslCaCert:        "/.secrets/tls/ca.crt",
		ClientApiHost:    cfg.ClientApiHost,
		ClientAccesToken: cfg.ClientAccessToken,
		MailUploaderName: cfg.MailUploaderName,
		MailUploader:     cfg.MailUploader,
		MailAddress:      cfg.MailAddress,
		MailPassword:     cfg.MailPassword,
		MailSmptHost:     cfg.MailSmtpHost,
		MailSmptPort:     strconv.Itoa(cfg.MailSmtpPort),
		CertSecretName:   cfg.CertSecretName,
		DbSecretName:     cfg.DbSecretName,
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
