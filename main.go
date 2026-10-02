package main

import (
	"log/slog"
	"os"

	rootCmd "github.com/NBISweden/sda-bpctl/cmd"
	_ "github.com/NBISweden/sda-bpctl/helpers"
	_ "github.com/NBISweden/sda-bpctl/internal/accession"
	_ "github.com/NBISweden/sda-bpctl/internal/dataset"
	_ "github.com/NBISweden/sda-bpctl/internal/ingest"
	_ "github.com/NBISweden/sda-bpctl/internal/job"
	_ "github.com/NBISweden/sda-bpctl/internal/landingpage"
	_ "github.com/NBISweden/sda-bpctl/internal/mail"
	_ "github.com/NBISweden/sda-bpctl/internal/status"
)

var version = "dev"

func main() {
	rootCmd.AddVersion(version)
	slog.Info("running", "version", version)
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
