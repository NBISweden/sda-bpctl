package dataset

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/NBISweden/sda-bpctl/internal/models"
)

type mockClient struct {
	UserFiles           []models.FileInfo
	UserFilesWithPrefix []models.FileInfo
	Response            *http.Response
}

func (m *mockClient) GetUsersFiles() ([]models.FileInfo, error) {
	return m.UserFiles, nil
}

func (m *mockClient) GetUsersFilesWithPrefix() ([]models.FileInfo, error) {
	return m.UserFilesWithPrefix, nil
}

func (m *mockClient) PostFileIngest(data []byte) ([]byte, error) {
	response, err := json.Marshal(m.Response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

func (m *mockClient) PostFileAccession(payload []byte) ([]byte, error) {
	response, err := json.Marshal(m.Response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

func (m *mockClient) GetDataset(datasetID string) (*models.DatasetInfo, error) {
	return nil, nil
}

func (m *mockClient) PostDatasetCreate(payload []byte) ([]byte, error) {
	response, err := json.Marshal(m.Response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

func (m *mockClient) GetFilesWithStatus(status string) ([]models.FileInfo, error) {
	return m.UserFilesWithPrefix, nil
}

func (m *mockClient) WaitForStatus(target int, status string, interval time.Duration, timeout time.Duration) ([]models.FileInfo, error) {
	return nil, nil
}

func newMockClient(userID string, datasetFolder string) *mockClient {
	// data is mocked so that we expect 2 files to be included in the dataset
	mock := &mockClient{
		UserFilesWithPrefix: []models.FileInfo{
			{InboxPath: fmt.Sprintf("/%s/%s/file1.c4gh", userID, datasetFolder), Status: "verified"},
			{InboxPath: fmt.Sprintf("/%s/%s/file2.c4gh", userID, datasetFolder), Status: "verified"},
		},
		UserFiles: []models.FileInfo{
			{InboxPath: fmt.Sprintf("/%s/%s/file1.c4gh", userID, datasetFolder), Status: "verified"},
			{InboxPath: fmt.Sprintf("/%s/%s/file2.c4gh", userID, datasetFolder), Status: "verified"},
			{InboxPath: fmt.Sprintf("/%s/%s/file3.c4gh", userID, "DATASET_OTHER"), Status: "verified"},
		},
		Response: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString("ok"))},
	}
	return mock
}

func TestDataset(t *testing.T) {
	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}

	workingDirectory := filepath.Dir(ex)
	datasetFolder := "DATASET_TEST"
	datasetID := "aa-Dataset-test"
	userID := "testuser"
	expectedNrFiles := 2
	mock := newMockClient(userID, datasetFolder)

	t.Run("Test Dataset", func(t *testing.T) {
		datasetCmd.Flag("data-directory").Value.Set(workingDirectory)
		var files []models.FileInfo
		files, err := mock.GetUsersFilesWithPrefix()
		if err != nil {
			t.Error(err)
		}

		err = createStableIDsFile(datasetFolder, files)
		if err != nil {
			t.Error(err)
		}

		files, err = mock.GetFilesWithStatus("verified")
		if err != nil {
			t.Error(err)
		}

		nrFiles := len(files)
		if nrFiles != expectedNrFiles {
			t.Logf("recieved %d/%d paths for accessionIDs", nrFiles, expectedNrFiles)
			t.Fail()
		}

		err = createDataset(mock, datasetID, userID, files)
		if err != nil {
			t.Error(err)
		}
	})
}
