package config

import (
	"fmt"
	"log/slog"

	"github.com/spf13/viper"
)

type Config struct {
	DatasetFolder                string `mapstructure:"DATASET_FOLDER"`
	DatasetID                    string `mapstructure:"DATASET_ID"`
	UserID                       string `mapstructure:"USER_ID"`
	SslCaCert                    string `mapstructure:"SSL_CA_CERT"`
	Timeout                      int    `mapstructure:"JOB_TIMEOUT"`
	PollRate                     int    `mapstructure:"JOB_POLL_RATE"`
	ClientApiHost                string `mapstructure:"CLIENT_API_HOST"`
	ClientAccessToken            string `mapstructure:"CLIENT_ACCESS_TOKEN"`
	DbHost                       string `mapstructure:"DB_HOST"`
	DbPort                       int    `mapstructure:"DB_PORT"`
	DbUser                       string `mapstructure:"DB_USER"`
	DbPassword                   string `mapstructure:"DB_PASSWORD"`
	DbName                       string `mapstructure:"DB_NAME"`
	DbSchema                     string `mapstructure:"DB_SCHEMA"`
	DbSslMode                    string `mapstructure:"DB_SSL_MODE"`
	DbClientCert                 string `mapstructure:"DB_CLIENT_CERT"`
	DbClientKey                  string `mapstructure:"DB_CLIENT_KEY"`
	MailAddress                  string `mapstructure:"MAIL_ADDRESS"`
	MailPassword                 string `mapstructure:"MAIL_PASSWORD"`
	MailSmtpHost                 string `mapstructure:"MAIL_SMTP_HOST"`
	MailSmtpPort                 int    `mapstructure:"MAIL_SMTP_PORT"`
	MailUploaderName             string `mapstructure:"MAIL_UPLOADER_NAME"`
	MailUploaderOrganizationName string `mapstructure:"MAIL_UPLOADER_ORGANIZATION_NAME"`
	MailUploader                 string `mapstructure:"MAIL_UPLOADER"`
}

func NewConfig(configPath string) (*Config, error) {
	v := viper.New()

	v.SetConfigFile(configPath)
	bindKeys(v)

	v.SetDefault("JOB_TIMEOUT", 4320)
	v.SetDefault("JOB_POLL_RATE", 180)

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			slog.Info("config file not found, using environment only")
		} else {
			slog.Warn("failed to read config file, falling back to environment variables", "err", err)
		}
	}

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("could not unmarshal config: %w", err)
	}

	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

func bindKeys(v *viper.Viper) {
	v.BindEnv("DATASET_FOLDER")
	v.BindEnv("DATASET_ID")
	v.BindEnv("USER_ID")
	v.BindEnv("SSL_CA_CERT")
	v.BindEnv("JOB_TIMEOUT")
	v.BindEnv("JOB_POLL_RATE")
	v.BindEnv("CLIENT_API_HOST")
	v.BindEnv("CLIENT_ACCESS_TOKEN")
	v.BindEnv("DB_HOST")
	v.BindEnv("DB_PORT")
	v.BindEnv("DB_USER")
	v.BindEnv("DB_PASSWORD")
	v.BindEnv("DB_NAME")
	v.BindEnv("DB_SCHEMA")
	v.BindEnv("DB_SSL_MODE")
	v.BindEnv("DB_CLIENT_CERT")
	v.BindEnv("DB_CLIENT_KEY")
	v.BindEnv("MAIL_ADDRESS")
	v.BindEnv("MAIL_PASSWORD")
	v.BindEnv("MAIL_SMTP_HOST")
	v.BindEnv("MAIL_SMTP_PORT")
	v.BindEnv("MAIL_UPLOADER_NAME")
	v.BindEnv("MAIL_UPLOADER_ORGANIZATION_NAME")
	v.BindEnv("MAIL_UPLOADER")
}

func validateConfig(cfg *Config) error {
	if cfg.DatasetFolder == "" {
		return fmt.Errorf("DATASET_FOLDER requiered")
	}

	if cfg.DatasetID == "" {
		return fmt.Errorf("DATASET_ID requiered")
	}

	if cfg.UserID == "" {
		return fmt.Errorf("USER_ID requiered")
	}

	if cfg.PollRate > cfg.Timeout {
		return fmt.Errorf("JOB_POLL_RATE greater than JOB_TIMEOUT, set a pollrate that is less than the timeout value")
	}
	return nil
}
