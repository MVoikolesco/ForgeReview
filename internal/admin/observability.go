package admin

import (
	"bufio"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type reviewLogSummary struct {
	Name      string `json:"name"`
	UpdatedAt string `json:"updated_at"`
	Files     int    `json:"files"`
	Bytes     int64  `json:"bytes"`
}

func (h Handler) observabilityRoute(w http.ResponseWriter, r *http.Request, action string) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	switch action {
	case "metrics":
		h.operationalMetrics(w, r)
	case "logs":
		h.workerLogs(w, r)
	case "reviews":
		h.reviewLogs(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h Handler) operationalMetrics(w http.ResponseWriter, r *http.Request) {
	response := map[string]any{"queue": map[string]any{"connected": false, "stream_length": 0, "pending": 0, "workers": []any{}}, "reviews": map[string]any{"total": 0, "bytes": 0}}
	if h.observer != nil {
		metrics, err := h.observer.Metrics(r.Context())
		if err != nil {
			response["queue_error"] = err.Error()
		} else {
			response["queue"] = metrics
		}
	}
	reviews, _ := scanReviewLogs(h.logDir)
	var bytes int64
	for _, item := range reviews {
		bytes += item.Bytes
	}
	response["reviews"] = map[string]any{"total": len(reviews), "bytes": bytes}
	writeJSON(w, 200, response)
}

func (h Handler) workerLogs(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("lines"))
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	path := filepath.Join(filepath.Dir(h.logDir), "worker.log")
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		writeJSON(w, 200, map[string]any{"lines": []string{}, "available": false})
		return
	}
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	defer file.Close()
	lines := []string{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > limit {
			lines = lines[len(lines)-limit:]
		}
	}
	if err := scanner.Err(); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"lines": lines, "available": true})
}

func (h Handler) reviewLogs(w http.ResponseWriter, r *http.Request) {
	reviews, err := scanReviewLogs(h.logDir)
	if err != nil && !os.IsNotExist(err) {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, reviews)
}

func scanReviewLogs(root string) ([]reviewLogSummary, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	items := []reviewLogSummary{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		summary := reviewLogSummary{Name: entry.Name()}
		_ = filepath.WalkDir(filepath.Join(root, entry.Name()), func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if d.IsDir() {
				return nil
			}
			info, infoErr := d.Info()
			if infoErr == nil {
				summary.Files++
				summary.Bytes += info.Size()
				if summary.UpdatedAt == "" || strings.Compare(info.ModTime().UTC().Format("2006-01-02T15:04:05Z"), summary.UpdatedAt) > 0 {
					summary.UpdatedAt = info.ModTime().UTC().Format("2006-01-02T15:04:05Z")
				}
			}
			return nil
		})
		items = append(items, summary)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt > items[j].UpdatedAt })
	if len(items) > 50 {
		items = items[:50]
	}
	return items, nil
}
