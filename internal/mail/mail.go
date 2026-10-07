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
	gomail "github.com/wneessen/go-mail"
)

//go:embed templates/*.html
var templateFS embed.FS
var dryRun bool
var configPath string
var DataDirectory string

var mailCmd = &cobra.Command{
	Use:   "mail",
	Short: "Send mail notifications",
	Long:  "Send mail notifications",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.NewConfig(configPath)
		if err != nil {
			return err
		}
		err = Run(cfg)
		if err != nil {
			return err
		}
		return nil
	},
}

func init() {
	cmd.AddCommand(mailCmd)
	mailCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Toggles dry-run mode. Dry run renders all mails and checks their attachments, but does not send them")
	mailCmd.Flags().StringVarP(&configPath, "config", "c", "config.yaml", "Path to configuration file")
	mailCmd.Flags().StringVarP(&DataDirectory, "data-directory", "d", "data", "Directory to retrieve files from to attach in mail notifications")
}

func Run(cfg *config.Config) error {
	m := New(cfg)
	for _, recipient := range []string{"BigPicture", "Minttu", "Submitter"} {
		if err := m.Notify(recipient, dryRun); err != nil {
			return fmt.Errorf("failed to notify %s: %w", recipient, err)
		}
	}
	return nil
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
	Uploader         string
	OrganizationName string
	DatasetID        string
	DatasetFolder    string
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
		smtpHost: c.MailSMTPHost,
		smtpPort: c.MailSMTPPort,
		email:    c.MailAddress,
		password: c.MailPassword,
		from:     c.MailAddress,
		data: TemplateData{
			Uploader:         c.MailUploaderName,
			OrganizationName: c.MailUploaderOrganizationName,
			DatasetID:        c.DatasetID,
			DatasetFolder:    c.DatasetFolder,
		},
		lookup: map[string]Notifiers{
			"Submitter": {
				email:       c.MailUploader,
				template:    "notify-submitter.html",
				subject:     "Successful Ingestion of Your Dataset Submission",
				attachments: []string{fmt.Sprintf("%s/%s-stableIDs.txt", DataDirectory, c.DatasetFolder)},
			},
			"BigPicture": {
				email:       "submit@bigpicture.eu",
				template:    "notify-bigpicture.html",
				subject:     fmt.Sprintf("Dataset %s has been ingested", c.DatasetID),
				attachments: []string{fmt.Sprintf("%s/xml/dataset.txt", DataDirectory), fmt.Sprintf("%s/xml/policy.txt", DataDirectory)},
			},
			"Minttu": {
				email:       "minttu.sauramo@hus.fi",
				cc:          []string{"jarno.laitinen@csc.fi"},
				template:    "notify-minttu.html",
				subject:     fmt.Sprintf("Dataset %s has been ingested", c.DatasetID),
				attachments: []string{fmt.Sprintf("%s/xml/dataset.txt", DataDirectory), fmt.Sprintf("%s/xml/rems.txt", DataDirectory), fmt.Sprintf("%s/xml/policy.txt", DataDirectory)},
			},
		},
	}
	return m
}

func (mail *Mail) Notify(notifier string, dryRun bool) error {
	htmlBody, err := renderTemplate(mail.lookup[notifier].template, mail.data)
	if err != nil {
		return fmt.Errorf("failed to render mail template: %v", err)
	}

	err = mail.send(mail.lookup[notifier].subject, htmlBody, mail.lookup[notifier].email, mail.lookup[notifier].attachments, mail.lookup[notifier].cc)
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

func (mail *Mail) send(subject, message, receiver string, attachments, ccs []string) error {
	m := gomail.NewMsg()

	if err := m.From(mail.from); err != nil {
		return err
	}

	if err := m.To(receiver); err != nil {
		return err
	}

	m.Subject(subject)

	if len(ccs) > 0 {
		if err := m.Cc(ccs...); err != nil {
			return err
		}
	}

	err := m.Bcc(mail.from)
	if err != nil {
		return err
	}

	m.SetBodyString("text/html", message)

	// Enforce that the wanted attachments are files that exists
	if err := attachmentsExists(attachments); err != nil {
		return err
	}

	for _, file := range attachments {
		m.AttachFile(file)
	}

	client, err := gomail.NewClient(mail.smtpHost, gomail.WithPort(mail.smtpPort), gomail.WithSMTPAuth(gomail.SMTPAuthPlain), gomail.WithUsername(mail.email), gomail.WithPassword(mail.password))
	if err != nil {
		return err
	}
	slog.Info("sending mail", "receiver", receiver)
	if dryRun {
		slog.Info("dry run enabled, no mail sent")
		return nil
	}

	return client.DialAndSend(m)
}

func attachmentsExists(attachments []string) error {
	for _, attachment := range attachments {
		info, err := os.Stat(attachment)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("file does not exist: %s", attachment)
			}
			return fmt.Errorf("error checking file %s: %w", attachment, err)
		}

		if info.IsDir() {
			return fmt.Errorf("path is a directory, not a file: %s", attachment)
		}

		if info.Size() == 0 {
			return fmt.Errorf("file is empty: %s", attachment)
		}
	}

	return nil
}
