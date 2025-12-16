package mail

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"log/slog"
	"os"

	"github.com/NBISweden/sda-bpctl/cmd"
	"github.com/NBISweden/sda-bpctl/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/gomail.v2"
)

//go:embed templates/*.html
var templateFS embed.FS
var dryRun bool
var configPath string
var dataDirectory string

var mailCmd = &cobra.Command{
	Use:   "mail",
	Short: "Send mail notifications",
	Long:  "Send mail notifications",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.NewConfig(configPath)
		if err != nil {
			return err
		}
		m := New(cfg)
		for _, recipient := range []string{"BigPicture", "Minttu", "Submitter"} {
			if err := m.Notify(recipient, dryRun); err != nil {
				return fmt.Errorf("failed to notify %s: %w", recipient, err)
			}
		}

		return nil
	},
}

func init() {
	cmd.AddCommand(mailCmd)
	mailCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Toggles dry-run mode. Dry run will send all emails to the address in configuration.Email (env or yaml conf)")
	mailCmd.Flags().StringVar(&configPath, "config", "config.yaml", "Path to configuration file")
	mailCmd.Flags().StringVar(&dataDirectory, "data-directory", "data", "Directory to retrieve files from to attach in mail notifications")
}

type Mail struct {
	smtpHost string
	smtpPort int
	email    string
	password string
	from     string
	data     TemplateData
	lookup   map[string]Notifiers
}

type TemplateData struct {
	Uploader      string
	DatasetID     string
	DatasetFolder string
}

type Notifiers struct {
	email       string
	cc          []string
	template    string
	subject     string
	attachments []string
}

func New(c *config.Config) *Mail {
	m := &Mail{
		smtpHost: c.MailSmtpHost,
		smtpPort: c.MailSmtpPort,
		email:    c.MailAddress,
		password: c.MailPassword,
		from:     c.MailAddress,
		data: TemplateData{
			Uploader:      c.MailUploaderName,
			DatasetID:     c.DatasetID,
			DatasetFolder: c.DatasetFolder,
		},
		lookup: map[string]Notifiers{
			"Submitter": {
				email:       c.MailUploader,
				template:    "notify-submitter.html",
				subject:     "Successful Ingestion of Your Dataset Submission",
				attachments: []string{fmt.Sprintf("%s/%s-stableIDs.txt", dataDirectory, c.DatasetFolder)},
			},
			"BigPicture": {
				email:       "submit@bigpicture.eu",
				template:    "notify-bigpicture.html",
				subject:     fmt.Sprintf("Dataset %s has been ingested", c.DatasetFolder),
				attachments: []string{fmt.Sprintf("%s/dataset.txt", dataDirectory), fmt.Sprintf("%s/policy.txt", dataDirectory)},
			},
			"Minttu": {
				email:       "minttu.sauramo@hus.fi",
				cc:          []string{"jarno.laitinen@csc.fi"},
				template:    "notify-minttu.html",
				subject:     fmt.Sprintf("Dataset %s has been ingested", c.DatasetFolder),
				attachments: []string{fmt.Sprintf("%s/dataset.txt", dataDirectory), fmt.Sprintf("%s/rems.txt", dataDirectory), fmt.Sprintf("%s/policy.txt", dataDirectory)},
			},
		},
	}
	return m
}

func (mail *Mail) send(subject string, message string, reciever string, attachements []string, ccs []string) error {
	m := gomail.NewMessage()
	m.SetHeader("From", mail.from)
	m.SetHeader("To", reciever)
	m.SetHeader("Subject", subject)

	if len(ccs) > 0 {
		addresses := make([]string, 0, len(ccs))
		for _, email := range ccs {
			addresses = append(addresses, m.FormatAddress(email, ""))
		}
		m.SetHeader("Cc", addresses...)
	}

	m.SetBody("text/html", message)

	// Enforce that the wanted attachements are files that exists
	if err := attachementsExists(attachements); err != nil {
		return err
	}
	for _, file := range attachements {
		m.Attach(file)
	}

	d := gomail.NewDialer(mail.smtpHost, mail.smtpPort, mail.email, mail.password)
	slog.Info("[mail] notification sent about dataset completion", "reciever", reciever)
	return d.DialAndSend(m)
}

func (mail *Mail) Notify(notifier string, dryRun bool) error {
	htmlBody, err := renderTemplate(mail.lookup[notifier].template, mail.data)
	if err != nil {
		return fmt.Errorf("failed to render mail template: %v", err)
	}

	if dryRun {
		slog.Info(fmt.Sprintf("[mail] dry-run enabled, using <%s> instead of <%s>", mail.email, mail.lookup[notifier].email))
		err = mail.send(mail.lookup[notifier].subject, htmlBody, mail.email, mail.lookup[notifier].attachments, nil)
	}

	if !dryRun {
		err = mail.send(mail.lookup[notifier].subject, htmlBody, mail.lookup[notifier].email, mail.lookup[notifier].attachments, mail.lookup[notifier].cc)
	}
	if err != nil {
		return fmt.Errorf("failed to send mail notification %v", err)
	}

	return nil
}

func renderTemplate(filename string, data TemplateData) (string, error) {
	tmpl, err := template.ParseFS(templateFS, "templates/"+filename)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func attachementsExists(attachements []string) error {
	for _, attachement := range attachements {
		info, err := os.Stat(attachement)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("file does not exist: %s", attachement)
			}
			return fmt.Errorf("error checking file %s: %w", attachement, err)
		}

		if info.IsDir() {
			return fmt.Errorf("path is a directory, not a file: %s", attachement)
		}

		if info.Size() == 0 {
			return fmt.Errorf("file is empty: %s", attachement)
		}
	}

	return nil
}
