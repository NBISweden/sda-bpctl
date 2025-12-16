package client

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
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

type Client struct {
	accessToken   string
	apiHost       string
	userID        string
	datasetFolder string
	datasetID     string
	httpClient    *http.Client
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
		apiHost:       cfg.ClientApiHost,
		userID:        cfg.UserID,
		datasetFolder: cfg.DatasetFolder,
		datasetID:     cfg.DatasetID,
		httpClient:    httpClient,
	}

	return client, nil
}

func (c *Client) GetUsersFilesWithPrefix() ([]byte, error) {
	basePath := fmt.Sprintf("users/%s/files", c.userID)

	u, err := url.Parse(basePath)
	if err != nil {
		return nil, fmt.Errorf("unable to parse base path: %w", err)
	}

	q := u.Query()
	q.Set("path_prefix", c.datasetFolder)
	u.RawQuery = q.Encode()

	return c.doRequest("GET", u.String(), nil)
}

func (c *Client) GetUsersFiles() ([]models.FileInfo, error) {
	respBody, err := c.doRequest("GET", fmt.Sprintf("users/%s/files", c.userID), nil)
	if err != nil {
		return nil, err
	}

	var files []models.FileInfo
	err = json.Unmarshal(respBody, &files)
	if err != nil {
		return nil, err
	}
	return files, nil
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

func (c *Client) doRequest(method, path string, body []byte) ([]byte, error) {
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

		if resp.StatusCode == http.StatusInternalServerError {
			resp.Body.Close()
			return fmt.Errorf("non-ok response from api: %s", resp.Status)
		}

		return nil

	}, backoff.NewExponentialBackOff())

	if err != nil {
		slog.Error("could not complete request", "err", err)
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("non-ok response: %s", resp.Status)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("could not read response body", "err", err)
		return nil, err
	}

	slog.Info("respone", "status", resp.Status, "code", resp.StatusCode)
	return responseBody, nil
}

func (c *Client) WaitForAccession(target int, interval time.Duration, timeout time.Duration) ([]string, error) {
	deadline := time.Now().Add(timeout)
	for {
		paths, err := c.getVerifiedFilePaths()
		if err != nil {
			return nil, err
		}

		if len(paths) >= target {
			return paths, nil
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timeout reached, only got %d/%d files", len(paths), target)
		}
		slog.Info(fmt.Sprintf("found %d/%d files - waiting: internal: %s timeout: %s", len(paths), target, interval, timeout))
		time.Sleep(interval)
	}
}

func (c *Client) getVerifiedFilePaths() ([]string, error) {
	files, err := c.GetUsersFiles()
	if err != nil {
		return nil, err
	}

	var paths []string
	for _, f := range files {
		if f.Status == "verified" &&
			strings.Contains(f.InboxPath, c.datasetFolder) &&
			!strings.Contains(f.InboxPath, "PRIVATE") {
			paths = append(paths, f.InboxPath)
		}
	}
	return paths, nil
}
