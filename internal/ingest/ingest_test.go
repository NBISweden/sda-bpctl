package ingest

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/NBISweden/sda-bpctl/internal/models"
)

type mockClient struct {
	FilesToReturn []models.FileInfo
	Response      []byte
	CallIndex     int

	requestedStatus string
	ingestedPaths   []string
}

func (m *mockClient) GetUsersFilesWithPrefix() ([]models.FileInfo, error) {
	return m.FilesToReturn, nil
}

func (m *mockClient) PostFileIngest(data []byte) ([]byte, error) {
	var payload map[string]string
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	m.ingestedPaths = append(m.ingestedPaths, payload["filepath"])
	return m.Response, nil
}

func (m *mockClient) PostFileAccession(payload []byte) ([]byte, error) {
	return m.Response, nil
}

func (m *mockClient) GetDataset(datasetID string) (*models.DatasetInfo, error) {
	return nil, nil
}

func (m *mockClient) PostDatasetCreate(payload []byte) ([]byte, error) {
	return m.Response, nil
}

// GetFilesWithStatus only filters on status; the dataset folder, PRIVATE and
// LANDING_PAGE filtering is done by the real client and tested there.
func (m *mockClient) GetFilesWithStatus(status string) ([]models.FileInfo, error) {
	m.requestedStatus = status
	var files []models.FileInfo
	for _, f := range m.FilesToReturn {
		if f.Status == status {
			files = append(files, f)
		}
	}
	return files, nil
}

func (m *mockClient) WaitForStatus(target int, status string, interval time.Duration, timeout time.Duration) ([]models.FileInfo, error) {
	return nil, nil
}

func setup(userID string, datasetFolder string) *mockClient {
	mock := &mockClient{
		FilesToReturn: []models.FileInfo{
			{InboxPath: fmt.Sprintf("/%s/%s/file1.c4gh", userID, datasetFolder), Status: "uploaded"},
			{InboxPath: fmt.Sprintf("/%s/%s/file2.c4gh", userID, datasetFolder), Status: "uploaded"},
			{InboxPath: fmt.Sprintf("/%s/%s/file3.c4gh", userID, datasetFolder), Status: "verified"},
			{InboxPath: fmt.Sprintf("/%s/%s/file5.c4gh", userID, datasetFolder), Status: "error"},
		},
		Response: []byte("ok"),
	}
	return mock
}

func TestIngest(t *testing.T) {
	userID := "testuser"
	datasetFolder := "DATASET_TEST"
	mock := setup(userID, datasetFolder)

	files, err := Run(mock, datasetFolder, userID)
	if err != nil {
		t.Fatal(err)
	}

	if mock.requestedStatus != "uploaded" {
		t.Errorf("requested files with status %q, want %q", mock.requestedStatus, "uploaded")
	}

	expectedPaths := []string{
		fmt.Sprintf("/%s/%s/file1.c4gh", userID, datasetFolder),
		fmt.Sprintf("/%s/%s/file2.c4gh", userID, datasetFolder),
	}
	if files != len(expectedPaths) {
		t.Errorf("ingested %d files, want %d", files, len(expectedPaths))
	}
	if len(mock.ingestedPaths) != len(expectedPaths) {
		t.Fatalf("posted %d ingest requests, want %d: %v", len(mock.ingestedPaths), len(expectedPaths), mock.ingestedPaths)
	}
	for i, path := range expectedPaths {
		if mock.ingestedPaths[i] != path {
			t.Errorf("ingest request %d: filepath = %q, want %q", i, mock.ingestedPaths[i], path)
		}
	}
}
