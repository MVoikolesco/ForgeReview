package admin

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gitea-agents/internal/review/pipeline"
	"gitea-agents/internal/reviewlog"
)

type reviewLogSummary struct {
	Name      string `json:"name"`
	UpdatedAt string `json:"updated_at"`
	Files     int    `json:"files"`
	Bytes     int64  `json:"bytes"`
	Owner     string `json:"owner,omitempty"`
	Repo      string `json:"repo,omitempty"`
	PRNumber  int    `json:"pr_number,omitempty"`
}

type reviewProgressSummary struct {
	Name      string                   `json:"name"`
	UpdatedAt string                   `json:"updated_at"`
	Percent   int                      `json:"percent"`
	Stage     string                   `json:"stage"`
	Status    string                   `json:"status"`
	Message   string                   `json:"message"`
	Events    []pipeline.ProgressEvent `json:"events"`
	Owner     string                   `json:"owner,omitempty"`
	Repo      string                   `json:"repo,omitempty"`
	PRNumber  int                      `json:"pr_number,omitempty"`
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
	case "progress":
		h.reviewProgress(w, r)
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
	var reviews []reviewLogSummary
	if h.logStore != nil {
		reviews, _ = h.reviewLogsFromStore(r.Context())
	} else {
		reviews, _ = scanReviewLogs(h.logDir)
	}
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
	if h.logStore != nil {
		reviews, err := h.reviewLogsFromStore(r.Context())
		if err != nil {
			writeError(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, reviews)
		return
	}
	reviews, err := scanReviewLogs(h.logDir)
	if err != nil && !os.IsNotExist(err) && !errors.Is(err, reviewlog.ErrNotFound) {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, reviews)
}

func (h Handler) reviewProgress(w http.ResponseWriter, r *http.Request) {
	var progress *reviewProgressSummary
	var err error
	if h.logStore != nil {
		progress, err = h.reviewProgressFromStore(r.Context(), r.URL.Query().Get("name"))
	} else {
		progress, err = reviewProgressFor(h.logDir, r.URL.Query().Get("name"))
	}
	if err != nil && !os.IsNotExist(err) {
		writeError(w, 500, err.Error())
		return
	}
	if progress == nil {
		writeJSON(w, 200, map[string]any{"active": false})
		return
	}
	writeJSON(w, 200, map[string]any{"active": progress.Status != "done" && progress.Status != "failed", "review": progress})
}

func (h Handler) reviewLogsFromStore(ctx context.Context) ([]reviewLogSummary, error) {
	names, err := h.logStore.Runs(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]reviewLogSummary, 0, len(names))
	for _, name := range names {
		progress, readErr := h.readProgressFromStore(ctx, name)
		if readErr != nil || progress == nil {
			continue
		}
		items = append(items, reviewLogSummary{Name: name, UpdatedAt: progress.UpdatedAt, Owner: progress.Owner, Repo: progress.Repo, PRNumber: progress.PRNumber})
	}
	return items, nil
}

func (h Handler) reviewProgressFromStore(ctx context.Context, name string) (*reviewProgressSummary, error) {
	if name != "" {
		return h.readProgressFromStore(ctx, filepath.Base(name))
	}
	names, err := h.logStore.Runs(ctx)
	if err != nil {
		return nil, err
	}
	for _, run := range names {
		progress, readErr := h.readProgressFromStore(ctx, run)
		if readErr == nil && progress != nil && len(progress.Events) > 0 {
			return progress, nil
		}
	}
	return nil, nil
}

func (h Handler) readProgressFromStore(ctx context.Context, name string) (*reviewProgressSummary, error) {
	content, err := h.logStore.Read(ctx, name, "00-progress.jsonl")
	if err != nil {
		return nil, err
	}
	return parseReviewProgress(content, name), nil
}

func reviewProgressFor(root, name string) (*reviewProgressSummary, error) {
	if name != "" {
		return readReviewProgress(filepath.Join(root, filepath.Base(name)), filepath.Base(name))
	}
	return latestReviewProgress(root)
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
		parts := strings.Split(entry.Name(), "_pr-")
		if len(parts) == 2 {
			prNumber := strings.SplitN(parts[1], "-run-", 2)[0]
			summary.PRNumber, _ = strconv.Atoi(prNumber)
			ownerRepo := strings.SplitN(parts[0], "_", 2)
			if len(ownerRepo) == 2 {
				summary.Owner, summary.Repo = ownerRepo[0], ownerRepo[1]
			}
		}
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

func latestReviewProgress(root string) (*reviewProgressSummary, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	items := []reviewProgressSummary{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		item, err := readReviewProgress(filepath.Join(root, entry.Name()), entry.Name())
		if err != nil || len(item.Events) == 0 {
			continue
		}
		items = append(items, *item)
	}
	if len(items) == 0 {
		return nil, nil
	}
	sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt > items[j].UpdatedAt })
	return &items[0], nil
}

func readReviewProgress(dir, name string) (*reviewProgressSummary, error) {
	path := filepath.Join(dir, "00-progress.jsonl")
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var content strings.Builder
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	content.Write(data)
	return parseReviewProgress(content.String(), name), nil
}

func parseReviewProgressContent(name string) *reviewProgressSummary {
	item := &reviewProgressSummary{Name: name, Events: []pipeline.ProgressEvent{}}
	parts := strings.Split(name, "_pr-")
	if len(parts) == 2 {
		prNumber := strings.SplitN(parts[1], "-run-", 2)[0]
		item.PRNumber, _ = strconv.Atoi(prNumber)
		ownerRepo := strings.SplitN(parts[0], "_", 2)
		if len(ownerRepo) == 2 {
			item.Owner, item.Repo = ownerRepo[0], ownerRepo[1]
		}
	}
	return item
}

func parseReviewProgress(content, name string) *reviewProgressSummary {
	item := parseReviewProgressContent(name)
	scanner := bufio.NewScanner(strings.NewReader(content))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event pipeline.ProgressEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		item.Events = append(item.Events, event)
		item.UpdatedAt = event.Timestamp
		item.Percent = event.Percent
		item.Stage = event.Stage
		item.Status = event.Status
		item.Message = event.Message
	}
	return item
}
