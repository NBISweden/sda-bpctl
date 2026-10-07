package client

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/NBISweden/sda-bpctl/internal/config"
	"github.com/NBISweden/sda-bpctl/internal/models"
	"github.com/cenkalti/backoff/v4"
)

// ErrNotFound is returned when the API responds with 404 Not Found.
var ErrNotFound = errors.New("not found")

type Client struct {
	accessToken   string
	apiHost       string
	userID        string
	datasetFolder string
	datasetID     string
	httpClient    *http.Client
}

var retryableStatusCodes = map[int]bool{
	http.StatusBadGateway:          true,
	http.StatusGatewayTimeout:      true,
	http.StatusInternalServerError: true,
	http.StatusServiceUnavailable:  true,
	// as of (2026-01-21) there is reason to beleve that the API can return erroneous responses of `400 bad request` that we wish to retry on
	http.StatusBadRequest: true,
}

func New(cfg *config.Config) (*Client, error) {
	httpClient := http.DefaultClient
	if cfg.SslCaCert != "" {
		caCert, err := os.ReadFile(cfg.SslCaCert)
		if err != nil {
			return nil, fmt.Errorf("init config: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if ok := caCertPool.AppendCertsFromPEM(caCert); !ok {
			return nil, fmt.Errorf("read CA cert %q: %w", cfg.SslCaCert, err)
		}

		tr := &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: caCertPool,
			},
		}
		httpClient = &http.Client{Transport: tr}
	}

	client := &Client{
		accessToken:   cfg.ClientAccessToken,
		apiHost:       cfg.ClientAPIHost,
		userID:        cfg.UserID,
		datasetFolder: cfg.DatasetFolder,
		datasetID:     cfg.DatasetID,
		httpClient:    httpClient,
	}

	return client, nil
}

// GetUsersFilesWithPrefix fetches every page of GET /users/<user>/files for
// the dataset folder. The API paginates with an opaque keyset cursor: each
// response carries an X-Next-Cursor header while more pages remain, which is
// passed back as the cursor query parameter until the header is absent.
func (c *Client) GetUsersFilesWithPrefix() ([]models.FileInfo, error) {
	basePath := fmt.Sprintf("users/%s/files", c.userID)

	u, err := url.Parse(basePath)
	if err != nil {
		return nil, fmt.Errorf("unable to parse base path: %w", err)
	}

	var allFiles []models.FileInfo
	seenCursors := make(map[string]bool)
	cursor := ""
	for {
		q := url.Values{}
		q.Set("path_prefix", c.datasetFolder)
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		u.RawQuery = q.Encode()

		respBody, header, err := c.doRequestWithHeader("GET", u.String(), nil)
		if err != nil {
			return nil, err
		}

		var files []models.FileInfo
		err = json.Unmarshal(respBody, &files)
		if err != nil {
			return nil, err
		}
		allFiles = append(allFiles, files...)

		cursor = header.Get("X-Next-Cursor")
		if cursor == "" {
			return allFiles, nil
		}
		if seenCursors[cursor] {
			return nil, fmt.Errorf("pagination cursor %q returned twice, aborting to avoid an endless loop", cursor)
		}
		seenCursors[cursor] = true
	}
}

func (c *Client) PostFileIngest(payload []byte) ([]byte, error) {
	return c.doRequest("POST", "file/ingest", payload)
}

func (c *Client) PostFileAccession(payload []byte) ([]byte, error) {
	return c.doRequest("POST", "file/accession", payload)
}

func (c *Client) PostDatasetCreate(payload []byte) ([]byte, error) {
	return c.doRequest("POST", "dataset/create", payload)
}

// GetDataset returns the state of a dataset, or nil if the dataset does not
// exist yet.
func (c *Client) GetDataset(datasetID string) (*models.DatasetInfo, error) {
	respBody, err := c.doRequest("GET", fmt.Sprintf("dataset/%s", url.PathEscape(datasetID)), nil)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}

	var dataset models.DatasetInfo
	err = json.Unmarshal(respBody, &dataset)
	if err != nil {
		return nil, err
	}
	return &dataset, nil
}

func (c *Client) doRequest(method, path string, body []byte) ([]byte, error) {
	responseBody, _, err := c.doRequestWithHeader(method, path, body)
	return responseBody, err
}

// doRequestWithHeader is doRequest but also returns the response headers.
func (c *Client) doRequestWithHeader(method, path string, body []byte) ([]byte, http.Header, error) {
	url := fmt.Sprintf("%s/%s", c.apiHost, path)

	var (
		resp *http.Response
		err  error
	)

	err = backoff.Retry(func() error {
		req, err := http.NewRequest(method, url, bytes.NewReader(body))
		if err != nil {
			slog.Warn("client new request err", "err", err)
			return err
		}

		req.Header.Set("Authorization", "Bearer "+c.accessToken)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		slog.Info("request", "method", method, "url", url)

		resp, err = c.httpClient.Do(req)
		if err != nil {
			slog.Warn("client do err", "err", err)
			return err
		}

		if retryableStatusCodes[resp.StatusCode] {
			resp.Body.Close()
			return fmt.Errorf("non-ok response from api: %s", resp.Status)
		}

		return nil

	}, backoff.NewExponentialBackOff())

	if err != nil {
		slog.Error("could not complete request", "err", err)
		return nil, nil, err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			return nil, nil, fmt.Errorf("%w: %s", ErrNotFound, resp.Status)
		}
		return nil, nil, fmt.Errorf("non-ok response: %s", resp.Status)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("could not read response body", "err", err)
		return nil, nil, err
	}

	slog.Info("respone", "status", resp.Status, "code", resp.StatusCode)
	return responseBody, resp.Header, nil
}

func (c *Client) WaitForStatus(target int, status string, interval time.Duration, timeout time.Duration) ([]models.FileInfo, error) {
	deadline := time.Now().Add(timeout)
	for {

		filteredFiles, err := c.GetFilesWithStatus(status)
		if err != nil {
			return nil, err
		}

		current := len(filteredFiles)

		if current >= target {
			return filteredFiles, nil
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timeout reached, only got %d/%d files", current, target)
		}
		slog.Info(fmt.Sprintf("found %d/%d files - waiting: %s timeout: %s", current, target, interval, timeout))
		time.Sleep(interval)
	}
}

func (c *Client) GetFilesWithStatus(status string) ([]models.FileInfo, error) {
	allFiles, err := c.GetUsersFilesWithPrefix()
	if err != nil {
		return nil, err
	}

	var filteredFiles []models.FileInfo
	for _, f := range allFiles {
		if f.Status == status &&
			strings.Contains(f.InboxPath, c.datasetFolder) &&
			!strings.Contains(f.InboxPath, "PRIVATE") &&
			!strings.Contains(f.InboxPath, "LANDING_PAGE") {
			filteredFiles = append(filteredFiles, f)
		}
	}
	return filteredFiles, nil
}
