package job

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/NBISweden/sda-bpctl/internal/models"
)

type mockClient struct {
	// UserFiles is returned by successive GetUsersDatasetFiles calls; the
	// last entry is repeated once exhausted.
	UserFiles [][]models.FileInfo
	// Datasets is returned by successive GetDataset calls in the same way.
	Datasets   []*models.DatasetInfo
	DatasetErr error

	userFilesCalls int
	datasetCalls   int
}

func (m *mockClient) GetUsersDatasetFiles() ([]models.FileInfo, error) {
	i := min(m.userFilesCalls, len(m.UserFiles)-1)
	m.userFilesCalls++
	return m.UserFiles[i], nil
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

func (m *mockClient) GetDataset(datasetID string) (*models.DatasetInfo, error) {
	if m.DatasetErr != nil {
		return nil, m.DatasetErr
	}
	if len(m.Datasets) == 0 {
		return nil, nil
	}
	i := min(m.datasetCalls, len(m.Datasets)-1)
	m.datasetCalls++
	return m.Datasets[i], nil
}

func (m *mockClient) GetFilesWithStatus(status string) ([]models.FileInfo, error) {
	return nil, nil
}

func (m *mockClient) WaitForStatus(target int, status string, interval time.Duration, timeout time.Duration) ([]models.FileInfo, error) {
	return nil, nil
}

const datasetFolder = "DATASET_TEST"

func datasetFiles(statuses ...string) []models.FileInfo {
	files := make([]models.FileInfo, len(statuses))
	for i, status := range statuses {
		files[i] = models.FileInfo{
			InboxPath: fmt.Sprintf("/testuser/%s/file%d.c4gh", datasetFolder, i),
			Status:    status,
		}
	}
	return files
}

func TestCountDatasetFiles(t *testing.T) {
	// files outside the dataset folder, PRIVATE and LANDING_PAGE are never counted
	other := []models.FileInfo{
		{InboxPath: "/testuser/DATASET_OTHER/file.c4gh", Status: "uploaded"},
		{InboxPath: "/testuser/" + datasetFolder + "/PRIVATE/file.c4gh", Status: "uploaded"},
		{InboxPath: "/testuser/" + datasetFolder + "/LANDING_PAGE/index.html.c4gh", Status: "uploaded"},
	}

	tests := []struct {
		name          string
		listed        []models.FileInfo
		dataset       *models.DatasetInfo
		wantNotMapped int
		wantMapped    int
	}{
		{
			name:          "fresh run, dataset does not exist yet",
			listed:        append(datasetFiles("uploaded", "uploaded", "uploaded"), other...),
			dataset:       nil,
			wantNotMapped: 3,
			wantMapped:    0,
		},
		{
			// mapped files are no longer returned by /users/<user>/files
			name:          "resumed run, some files already mapped",
			listed:        append(datasetFiles("ready", "ready"), other...),
			dataset:       &models.DatasetInfo{NumberOfFiles: 3},
			wantNotMapped: 2,
			wantMapped:    3,
		},
		{
			name:          "resumed run, all files already mapped",
			listed:        other,
			dataset:       &models.DatasetInfo{NumberOfFiles: 5},
			wantNotMapped: 0,
			wantMapped:    5,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mockClient{UserFiles: [][]models.FileInfo{tc.listed}}
			if tc.dataset != nil {
				mock.Datasets = []*models.DatasetInfo{tc.dataset}
			}

			notMapped, mapped, err := countDatasetFiles(mock, datasetFolder, "aa-Dataset-test")
			if err != nil {
				t.Fatal(err)
			}
			if notMapped != tc.wantNotMapped {
				t.Errorf("notMapped = %d, want %d", notMapped, tc.wantNotMapped)
			}
			if mapped != tc.wantMapped {
				t.Errorf("mapped = %d, want %d", mapped, tc.wantMapped)
			}
		})
	}
}

func TestCountDatasetFilesNoFiles(t *testing.T) {
	mock := &mockClient{UserFiles: [][]models.FileInfo{nil}}

	_, _, err := countDatasetFiles(mock, datasetFolder, "aa-Dataset-test")
	if err == nil {
		t.Fatal("expected an error when there are no files at all, got nil")
	}
}

func TestCountDatasetFilesDatasetError(t *testing.T) {
	wantErr := errors.New("api down")
	mock := &mockClient{
		UserFiles:  [][]models.FileInfo{datasetFiles("uploaded")},
		DatasetErr: wantErr,
	}

	_, _, err := countDatasetFiles(mock, datasetFolder, "aa-Dataset-test")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
}

func TestWaitForDatasetMapping(t *testing.T) {
	mock := &mockClient{Datasets: []*models.DatasetInfo{
		nil, // not created yet
		{NumberOfFiles: 2},
		{NumberOfFiles: 5},
	}}

	err := waitForDatasetMapping(mock, "aa-Dataset-test", 5, 0, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if mock.datasetCalls != 3 {
		t.Errorf("got %d GetDataset calls, want 3", mock.datasetCalls)
	}
}

func TestWaitForDatasetMappingTimeout(t *testing.T) {
	// a resumed run with an undercounted target would pass here early; the
	// target must include the already mapped files
	mock := &mockClient{Datasets: []*models.DatasetInfo{{NumberOfFiles: 3}}}

	err := waitForDatasetMapping(mock, "aa-Dataset-test", 5, 0, -time.Second)
	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "3/5") {
		t.Errorf("error should report progress 3/5, got: %v", err)
	}
}

func TestWaitForDatasetFiles(t *testing.T) {
	mock := &mockClient{UserFiles: [][]models.FileInfo{
		datasetFiles("uploaded", "uploaded", "uploaded"),
		datasetFiles("verified", "uploaded", "ready"),
		datasetFiles("verified", "verified", "ready"),
	}}

	err := waitForDatasetFiles(mock, datasetFolder, 3, []string{"verified", "ready"}, 0, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if mock.userFilesCalls != 3 {
		t.Errorf("got %d GetUsersDatasetFiles calls, want 3", mock.userFilesCalls)
	}
}

func TestWaitForDatasetFilesErrorStatus(t *testing.T) {
	mock := &mockClient{UserFiles: [][]models.FileInfo{
		datasetFiles("verified", "error", "verified"),
	}}

	err := waitForDatasetFiles(mock, datasetFolder, 3, []string{"verified"}, 0, time.Minute)
	if err == nil {
		t.Fatal("expected an error for a file in error status, got nil")
	}
	if mock.userFilesCalls != 1 {
		t.Errorf("should fail on the first poll, got %d calls", mock.userFilesCalls)
	}
}

func TestCheckStableIDsFile(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected int
		wantErr  string
	}{
		{
			name:     "complete list",
			content:  "aa-File-1 /DATASET_TEST/file1.c4gh\naa-File-2 /DATASET_TEST/file2.c4gh\naa-File-3 /DATASET_TEST/file3.c4gh\n",
			expected: 3,
		},
		{
			name:     "blank lines are not counted",
			content:  "aa-File-1 /DATASET_TEST/file1.c4gh\n\naa-File-2 /DATASET_TEST/file2.c4gh\n\n",
			expected: 2,
		},
		{
			// resumed run: only the files mapped in this run are listed
			name:     "incomplete list",
			content:  "aa-File-3 /DATASET_TEST/file3.c4gh\n",
			expected: 3,
			wantErr:  "lists 1 of 3 dataset files",
		},
		{
			name:     "more files than expected",
			content:  "aa-File-1 a\naa-File-2 b\naa-File-3 c\naa-File-4 d\n",
			expected: 3,
			wantErr:  "lists 4 of 3 dataset files",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "DATASET_TEST-stableIDs.txt")
			if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}

			err := checkStableIDsFile(path, tc.expected)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestCheckStableIDsFileMissing(t *testing.T) {
	// resumed run where all files were already mapped: no file is written
	path := filepath.Join(t.TempDir(), "DATASET_TEST-stableIDs.txt")

	err := checkStableIDsFile(path, 3)
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected a does-not-exist error, got %v", err)
	}
}
