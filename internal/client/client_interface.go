package client

import (
	"github.com/NBISweden/submitter/internal/models"
)

type APIClient interface {
	GetUsersFiles() ([]models.FileInfo, error)
	PostFileIngest([]byte) ([]byte, error)
	PostFileAccession(payload []byte) ([]byte, error)
}
