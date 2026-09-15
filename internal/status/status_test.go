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
			{FileID: "file1", InboxPath: fmt.Sprintf("/%s/%s/file1.c4gh", userID, datasetFolder), Status: "uploaded"},
			{FileID: "file2", InboxPath: fmt.Sprintf("/%s/%s/file2.c4gh", userID, datasetFolder), Status: "uploaded"},
			{FileID: "file3", InboxPath: fmt.Sprintf("/%s/%s/file3.c4gh", userID, datasetFolder), Status: "verified"},
			{FileID: "file4", InboxPath: fmt.Sprintf("/%s/%s/file4.c4gh", userID, datasetFolder), Status: "verified"},
			{FileID: "file5", InboxPath: fmt.Sprintf("/%s/%s/file5.c4gh", userID, datasetFolder), Status: "verified"},
			{FileID: "file6", InboxPath: fmt.Sprintf("/%s/%s/file6.c4gh", userID, datasetFolder), Status: "ready"},
			{FileID: "file7", InboxPath: fmt.Sprintf("/%s/PRIVATE/%s/file7.c4gh", userID, datasetFolder), Status: "uploaded"},
			{FileID: "file8", InboxPath: fmt.Sprintf("/%s/%s/LANDING_PAGE/file8.c4gh", userID, datasetFolder), Status: "ready"},
		},
	}
}

func TestRun(t *testing.T) {
	mock := setup("testuser", "DATASET_TEST")

	files, err := Run(mock)
	if err != nil {
		t.Fatal(err)
	}

	expectedFiles := 6 // excludes the PRIVATE and LANDING_PAGE entries
	if len(files) != expectedFiles {
		t.Errorf("got %d files, want %d", len(files), expectedFiles)
	}

	for _, f := range files {
		if f.FileID == "file7" || f.FileID == "file8" {
			t.Errorf("expected %s to be filtered out, but it was present", f.FileID)
		}
	}
}

func TestCountByStatus(t *testing.T) {
	mock := setup("testuser", "DATASET_TEST")
	files, err := Run(mock)
	if err != nil {
		t.Fatal(err)
	}

	counts := CountByStatus(files)

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

func TestFileIDsForStatus(t *testing.T) {
	mock := setup("testuser", "DATASET_TEST")
	files, err := Run(mock)
	if err != nil {
		t.Fatal(err)
	}

	ids := FileIDsForStatus(files, "verified")
	expected := []string{"file3", "file4", "file5"}

	if len(ids) != len(expected) {
		t.Fatalf("got %d ids, want %d: %v", len(ids), len(expected), ids)
	}
	for i, id := range expected {
		if ids[i] != id {
			t.Errorf("ids[%d] = %q, want %q", i, ids[i], id)
		}
	}

	if got := FileIDsForStatus(files, "nonexistent"); got != nil {
		t.Errorf("expected nil for unmatched status, got %v", got)
	}
}

func TestFormatSQLInClause(t *testing.T) {
	got := FormatSQLInClause([]string{"file3", "file4", "file5"})
	want := "('file3', 'file4', 'file5')"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	if got := FormatSQLInClause(nil); got != "()" {
		t.Errorf("got %q for empty input, want %q", got, "()")
	}

	got = FormatSQLInClause([]string{"a'b"})
	want = "('a''b')"
	if got != want {
		t.Errorf("got %q, want %q (single quote should be escaped)", got, want)
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
