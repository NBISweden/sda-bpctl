package status

import (
	"fmt"
	"testing"
	"time"

	"github.com/NBISweden/sda-bpctl/internal/models"
)

type mockClient struct {
	FilesToReturn []models.FileInfo
}

func (m *mockClient) GetUsersFiles() ([]models.FileInfo, error) {
	return m.FilesToReturn, nil
}

func (m *mockClient) GetUsersFilesWithPrefix() ([]models.FileInfo, error) {
	return m.FilesToReturn, nil
}

func (m *mockClient) PostFileIngest(data []byte) ([]byte, error) {
	return nil, nil
}

func (m *mockClient) PostFileAccession(payload []byte) ([]byte, error) {
	return nil, nil
}

func (m *mockClient) PostDatasetCreate(payload []byte) ([]byte, error) {
	return nil, nil
}

func (m *mockClient) GetFilesWithStatus(status string) ([]models.FileInfo, error) {
	return nil, nil
}

func (m *mockClient) WaitForStatus(target int, status string, interval time.Duration, timeout time.Duration) ([]models.FileInfo, error) {
	return nil, nil
}

func setup(userID string, datasetFolder string) *mockClient {
	return &mockClient{
		FilesToReturn: []models.FileInfo{
			{InboxPath: fmt.Sprintf("/%s/%s/file1.c4gh", userID, datasetFolder), Status: "uploaded"},
			{InboxPath: fmt.Sprintf("/%s/%s/file2.c4gh", userID, datasetFolder), Status: "uploaded"},
			{InboxPath: fmt.Sprintf("/%s/%s/file3.c4gh", userID, datasetFolder), Status: "verified"},
			{InboxPath: fmt.Sprintf("/%s/%s/file4.c4gh", userID, datasetFolder), Status: "verified"},
			{InboxPath: fmt.Sprintf("/%s/%s/file5.c4gh", userID, datasetFolder), Status: "verified"},
			{InboxPath: fmt.Sprintf("/%s/%s/file6.c4gh", userID, datasetFolder), Status: "ready"},
			{InboxPath: fmt.Sprintf("/%s/PRIVATE/%s/file7.c4gh", userID, datasetFolder), Status: "uploaded"},
			{InboxPath: fmt.Sprintf("/%s/%s/LANDING_PAGE/file8.c4gh", userID, datasetFolder), Status: "ready"},
		},
	}
}

func TestRun(t *testing.T) {
	mock := setup("testuser", "DATASET_TEST")

	counts, err := Run(mock)
	if err != nil {
		t.Fatal(err)
	}

	expected := map[string]int{"uploaded": 2, "verified": 3, "ready": 1}
	for status, want := range expected {
		if got := counts[status]; got != want {
			t.Errorf("status %q: got %d, want %d", status, got, want)
		}
	}
	if len(counts) != len(expected) {
		t.Errorf("got %d distinct statuses, want %d", len(counts), len(expected))
	}
}

func TestFormatReport(t *testing.T) {
	counts := map[string]int{"uploaded": 124, "submitted": 2, "verified": 2012}

	got := FormatReport(counts)
	want := "- verified: 2012\n- uploaded: 124\n- submitted: 2\n"

	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}
