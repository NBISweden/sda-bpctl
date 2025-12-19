package main

import (
	"log/slog"
	"os"

	"github.com/NBISweden/sda-bpctl/cmd"
	_ "github.com/NBISweden/sda-bpctl/helpers"
	_ "github.com/NBISweden/sda-bpctl/internal/accession"
	_ "github.com/NBISweden/sda-bpctl/internal/dataset"
	_ "github.com/NBISweden/sda-bpctl/internal/ingest"
	_ "github.com/NBISweden/sda-bpctl/internal/job"
	_ "github.com/NBISweden/sda-bpctl/internal/mail"
)

var version = "v1.1.1"

func main() {
	slog.Info("running", "version", version)
	err := cmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
