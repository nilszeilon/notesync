package sync

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nilszeilon/notesync/internal/storage"
)

type Client struct {
	serverURL  string
	token      string
	httpClient *http.Client
}

func NewClient(serverURL, token string) *Client {
	return &Client{
		serverURL: strings.TrimRight(serverURL, "/"),
		token:     token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) ListRemote() ([]storage.FileInfo, error) {
	req, err := http.NewRequest(http.MethodGet, c.serverURL+"/api/files", nil)
	if err != nil {
		return nil, err
	}
	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list remote: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list remote: %s - %s", resp.Status, string(body))
	}

	var files []storage.FileInfo
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return files, nil
}

func (c *Client) ListTombstones() ([]storage.Tombstone, error) {
	req, err := http.NewRequest(http.MethodGet, c.serverURL+"/api/tombstones", nil)
	if err != nil {
		return nil, err
	}
	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list tombstones: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list tombstones: %s - %s", resp.Status, string(body))
	}

	var tombstones []storage.Tombstone
	if err := json.NewDecoder(resp.Body).Decode(&tombstones); err != nil {
		return nil, fmt.Errorf("decode tombstones: %w", err)
	}
	return tombstones, nil
}

func (c *Client) Upload(relPath string, localPath string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	req, err := http.NewRequest(http.MethodPut, c.serverURL+"/api/files/"+relPath, f)
	if err != nil {
		return err
	}
	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload %s: %s - %s", relPath, resp.Status, string(body))
	}
	return nil
}

func (c *Client) Download(relPath, localPath string) error {
	req, err := http.NewRequest(http.MethodGet, c.serverURL+"/api/files/"+relPath, nil)
	if err != nil {
		return err
	}
	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("download %s: %s - %s", relPath, resp.Status, string(body))
	}

	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return fmt.Errorf("create parent dirs: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(localPath), ".notesync-dl-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write download: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpPath, localPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename download: %w", err)
	}
	return nil
}

func (c *Client) Delete(relPath string) error {
	req, err := http.NewRequest(http.MethodDelete, c.serverURL+"/api/files/"+relPath, nil)
	if err != nil {
		return err
	}
	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete %s: %s - %s", relPath, resp.Status, string(body))
	}
	return nil
}

func (c *Client) setAuth(req *http.Request) {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
}
