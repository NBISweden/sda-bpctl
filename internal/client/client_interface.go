package client

import (
	"github.com/NBISweden/sda-bpctl/internal/models"
	"time"
)

type APIClient interface {
	GetUsersFiles() ([]models.FileInfo, error)
	GetUsersFilesWithPrefix() ([]models.FileInfo, error)
	PostFileIngest([]byte) ([]byte, error)
	PostFileAccession(payload []byte) ([]byte, error)
	PostDatasetCreate(payload []byte) ([]byte, error)
	GetDataset(datasetID string) (*models.DatasetInfo, error)
	GetFilesWithStatus(status string) ([]models.FileInfo, error)
	WaitForStatus(target int, status string, interval time.Duration, timeout time.Duration) ([]models.FileInfo, error)
}
