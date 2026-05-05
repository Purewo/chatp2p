package httpapi

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultRecentDocLimit = 5
	maxRecentDocLimit     = 20
)

type recentDocsResponse struct {
	Items []recentDoc `json:"items"`
}

type recentDoc struct {
	Path      string    `json:"path"`
	Title     string    `json:"title"`
	UpdatedAt time.Time `json:"updatedAt"`
	Content   string    `json:"content"`
}

func (api *API) handleOpenAPIDocument(w http.ResponseWriter, r *http.Request) {
	root, err := repositoryRoot()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "documentation root was not found")
		return
	}

	body, err := os.ReadFile(filepath.Join(root, "api", "openapi.yaml"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "openapi document was not found")
		return
	}

	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func (api *API) handleRecentDocs(w http.ResponseWriter, r *http.Request) {
	root, err := repositoryRoot()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "documentation root was not found")
		return
	}

	limit := defaultRecentDocLimit
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		limit, err = strconv.Atoi(rawLimit)
		if err != nil || limit < 1 || limit > maxRecentDocLimit {
			writeError(w, http.StatusBadRequest, "invalid_request", "limit must be between 1 and 20")
			return
		}
	}

	docs, err := recentMarkdownDocs(root, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "documentation could not be read")
		return
	}

	writeJSON(w, http.StatusOK, recentDocsResponse{Items: docs})
}

func repositoryRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if fileExists(filepath.Join(dir, "api", "openapi.yaml")) && dirExists(filepath.Join(dir, "docs")) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("repository root not found")
}

func recentMarkdownDocs(root string, limit int) ([]recentDoc, error) {
	entries, err := os.ReadDir(filepath.Join(root, "docs"))
	if err != nil {
		return nil, err
	}

	docs := make([]recentDoc, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}

		absPath := filepath.Join(root, "docs", entry.Name())
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		body, err := os.ReadFile(absPath)
		if err != nil {
			return nil, err
		}

		relPath := filepath.ToSlash(filepath.Join("docs", entry.Name()))
		docs = append(docs, recentDoc{
			Path:      relPath,
			Title:     markdownTitle(body, strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))),
			UpdatedAt: info.ModTime().UTC(),
			Content:   string(body),
		})
	}

	sort.Slice(docs, func(i, j int) bool {
		if docs[i].UpdatedAt.Equal(docs[j].UpdatedAt) {
			return docs[i].Path < docs[j].Path
		}
		return docs[i].UpdatedAt.After(docs[j].UpdatedAt)
	})

	if len(docs) > limit {
		docs = docs[:limit]
	}
	return docs, nil
}

func markdownTitle(body []byte, fallback string) string {
	for _, line := range bytes.Split(body, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if title, ok := bytes.CutPrefix(line, []byte("# ")); ok {
			return string(bytes.TrimSpace(title))
		}
	}
	return fallback
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
