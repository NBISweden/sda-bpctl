package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/NBISweden/sda-bpctl/internal/models"
)

// fakeFilesAPI mimics the paginated GET /users/<user>/files endpoint: it
// returns at most pageSize files per request and sets X-Next-Cursor while
// more files remain.
type fakeFilesAPI struct {
	files    []models.FileInfo
	pageSize int

	mu       sync.Mutex
	requests []*http.Request
}

func (f *fakeFilesAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, r)
	f.mu.Unlock()

	if r.URL.Path != "/users/testuser/files" {
		http.NotFound(w, r)
		return
	}

	matching := f.files
	if prefix := r.URL.Query().Get("path_prefix"); prefix != "" {
		matching = nil
		for _, file := range f.files {
			if strings.HasPrefix(file.InboxPath, prefix) {
				matching = append(matching, file)
			}
		}
	}

	start := 0
	if cursor := r.URL.Query().Get("cursor"); cursor != "" {
		offset, err := strconv.Atoi(strings.TrimPrefix(cursor, "offset-"))
		if err != nil {
			http.Error(w, "bad cursor", http.StatusNotFound)
			return
		}
		start = offset
	}

	end := min(start+f.pageSize, len(matching))
	if end < len(matching) {
		w.Header().Set("X-Next-Cursor", fmt.Sprintf("offset-%d", end))
	}

	page := matching[start:end]
	if page == nil {
		page = []models.FileInfo{}
	}
	if err := json.NewEncoder(w).Encode(page); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func makeFiles(n int, folder string) []models.FileInfo {
	files := make([]models.FileInfo, n)
	for i := range files {
		files[i] = models.FileInfo{
			FileID:    fmt.Sprintf("%s-file%d", folder, i),
			InboxPath: fmt.Sprintf("%s/file%d.c4gh", folder, i),
			Status:    "uploaded",
		}
	}
	return files
}

func newTestClient(t *testing.T, handler http.Handler) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	return &Client{
		accessToken:   "token",
		apiHost:       server.URL,
		userID:        "testuser",
		datasetFolder: "DATASET_TEST",
		httpClient:    server.Client(),
	}
}

func assertFileIDs(t *testing.T, got, want []models.FileInfo) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d files, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].FileID != want[i].FileID {
			t.Errorf("files[%d] = %q, want %q", i, got[i].FileID, want[i].FileID)
		}
	}
}

func TestGetUsersDatasetFilesSinglePage(t *testing.T) {
	api := &fakeFilesAPI{files: makeFiles(3, "DATASET_TEST"), pageSize: 1000}
	c := newTestClient(t, api)

	files, err := c.GetUsersDatasetFiles()
	if err != nil {
		t.Fatal(err)
	}

	assertFileIDs(t, files, api.files)
	if len(api.requests) != 1 {
		t.Errorf("got %d requests, want 1", len(api.requests))
	}
	if got := api.requests[0].URL.Query().Get("cursor"); got != "" {
		t.Errorf("first request should not send a cursor, got %q", got)
	}
}

func TestGetUsersDatasetFilesFollowsCursor(t *testing.T) {
	// 2500 files with the API's default page size of 1000 -> 3 pages
	api := &fakeFilesAPI{files: makeFiles(2500, "DATASET_TEST"), pageSize: 1000}
	c := newTestClient(t, api)

	files, err := c.GetUsersDatasetFiles()
	if err != nil {
		t.Fatal(err)
	}

	assertFileIDs(t, files, api.files)

	wantCursors := []string{"", "offset-1000", "offset-2000"}
	if len(api.requests) != len(wantCursors) {
		t.Fatalf("got %d requests, want %d", len(api.requests), len(wantCursors))
	}
	for i, r := range api.requests {
		q := r.URL.Query()
		if got := q.Get("cursor"); got != wantCursors[i] {
			t.Errorf("request %d: cursor = %q, want %q", i, got, wantCursors[i])
		}
		if got := q.Get("path_prefix"); got != "DATASET_TEST" {
			t.Errorf("request %d: path_prefix = %q, want %q", i, got, "DATASET_TEST")
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Errorf("request %d: Authorization = %q, want %q", i, got, "Bearer token")
		}
	}
}

func TestGetUsersDatasetFilesExactPageBoundary(t *testing.T) {
	// exactly two full pages: no cursor on the last page, so no empty third request
	api := &fakeFilesAPI{files: makeFiles(2000, "DATASET_TEST"), pageSize: 1000}
	c := newTestClient(t, api)

	files, err := c.GetUsersDatasetFiles()
	if err != nil {
		t.Fatal(err)
	}

	assertFileIDs(t, files, api.files)
	if len(api.requests) != 2 {
		t.Errorf("got %d requests, want 2", len(api.requests))
	}
}

func TestGetUsersDatasetFilesNoFiles(t *testing.T) {
	api := &fakeFilesAPI{pageSize: 1000}
	c := newTestClient(t, api)

	files, err := c.GetUsersDatasetFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Errorf("got %d files, want 0", len(files))
	}
}

func TestGetUsersDatasetFilesOnlyDatasetFolder(t *testing.T) {
	wanted := makeFiles(5, "DATASET_TEST")
	all := append(makeFiles(4, "DATASET_OTHER"), wanted...)
	api := &fakeFilesAPI{files: all, pageSize: 2}
	c := newTestClient(t, api)

	files, err := c.GetUsersDatasetFiles()
	if err != nil {
		t.Fatal(err)
	}

	assertFileIDs(t, files, wanted)
	if len(api.requests) != 3 {
		t.Errorf("got %d requests, want 3", len(api.requests))
	}
}

func TestGetUsersDatasetFilesErrorOnLaterPage(t *testing.T) {
	api := &fakeFilesAPI{files: makeFiles(5, "DATASET_TEST"), pageSize: 2}
	// fail the second page with a non-retryable status so the test does not wait on backoff
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("cursor") != "" {
			api.mu.Lock()
			api.requests = append(api.requests, r)
			api.mu.Unlock()
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		api.ServeHTTP(w, r)
	})
	c := newTestClient(t, handler)

	files, err := c.GetUsersDatasetFiles()
	if err == nil {
		t.Fatal("expected an error when a later page fails, got nil")
	}
	if files != nil {
		t.Errorf("expected no partial result on error, got %d files", len(files))
	}
}

func TestGetUsersDatasetFilesRepeatedCursor(t *testing.T) {
	requests := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests > 10 {
			t.Error("client kept following a repeated cursor")
			http.Error(w, "too many requests", http.StatusForbidden)
			return
		}
		w.Header().Set("X-Next-Cursor", "same-cursor")
		_, _ = w.Write([]byte(`[]`))
	})
	c := newTestClient(t, handler)

	_, err := c.GetUsersDatasetFiles()
	if err == nil {
		t.Fatal("expected an error for a repeated cursor, got nil")
	}
	if !strings.Contains(err.Error(), "same-cursor") {
		t.Errorf("error should mention the repeated cursor, got: %v", err)
	}
	if requests != 2 {
		t.Errorf("got %d requests, want 2", requests)
	}
}

func TestDoRequestNotFound(t *testing.T) {
	c := newTestClient(t, http.NotFoundHandler())

	_, err := c.GetUsersDatasetFiles()
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetFilesWithStatus(t *testing.T) {
	files := []models.FileInfo{
		{FileID: "uploaded1", InboxPath: "DATASET_TEST/file1.c4gh", Status: "uploaded"},
		{FileID: "uploaded2", InboxPath: "DATASET_TEST/sub/file2.c4gh", Status: "uploaded"},
		{FileID: "verified", InboxPath: "DATASET_TEST/file3.c4gh", Status: "verified"},
		{FileID: "private", InboxPath: "DATASET_TEST/PRIVATE/file4.c4gh", Status: "uploaded"},
		{FileID: "landingpage", InboxPath: "DATASET_TEST/LANDING_PAGE/index.html.c4gh", Status: "uploaded"},
		{FileID: "otherfolder", InboxPath: "DATASET_OTHER/file5.c4gh", Status: "uploaded"},
	}
	api := &fakeFilesAPI{files: files, pageSize: 2}
	c := newTestClient(t, api)

	got, err := c.GetFilesWithStatus("uploaded")
	if err != nil {
		t.Fatal(err)
	}

	assertFileIDs(t, got, []models.FileInfo{{FileID: "uploaded1"}, {FileID: "uploaded2"}})
}
