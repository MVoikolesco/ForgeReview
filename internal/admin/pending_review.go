package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gitea-agents/internal/queue"
	"gitea-agents/internal/review"
	"gitea-agents/internal/review/pipeline"
	"gitea-agents/internal/reviewlog"
)

type pendingReviewDecision struct {
	Action string `json:"action"`
	At     string `json:"at"`
}

type pendingReviewResponse struct {
	Name        string                       `json:"name"`
	Job         queue.ReviewJob              `json:"job"`
	FinalReview review.FinalReview           `json:"final_review"`
	Publication pendingReviewPublicationView `json:"publication"`
}

type pendingReviewPublicationView struct {
	Event    string                     `json:"event"`
	Body     string                     `json:"body"`
	Comments []pendingReviewCommentView `json:"comments"`
}

type pendingReviewCommentView struct {
	Path        string `json:"path"`
	NewPosition int    `json:"new_position"`
	Body        string `json:"body"`
}

func (h Handler) pendingReviewRoute(w http.ResponseWriter, r *http.Request, action string) {
	if action == "" && r.Method == http.MethodGet {
		h.pendingReview(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	switch action {
	case "approve":
		h.approvePendingReview(w, r)
	case "reject":
		h.decidePendingReview(w, r, "reject")
	case "rerun":
		h.rerunPendingReview(w, r)
	default:
		http.NotFound(w, r)
	}
}

func pendingName(r *http.Request) string {
	name := filepath.Base(strings.TrimSpace(r.URL.Query().Get("name")))
	if name == "." || name == "" || name != r.URL.Query().Get("name") {
		return ""
	}
	return name
}

func (h Handler) readPendingReview(r *http.Request) (string, review.PendingReview, error) {
	name := pendingName(r)
	if name == "" {
		return "", review.PendingReview{}, fmt.Errorf("execução inválida")
	}
	var content string
	var err error
	if h.logStore != nil {
		content, err = h.logStore.Read(r.Context(), name, "pending-review.json")
	} else {
		path := filepath.Join(h.logDir, name, "pending-review.json")
		var bytes []byte
		bytes, err = os.ReadFile(path)
		content = string(bytes)
	}
	if err != nil {
		return "", review.PendingReview{}, err
	}
	var pending review.PendingReview
	if err := json.Unmarshal([]byte(content), &pending); err != nil {
		return "", review.PendingReview{}, fmt.Errorf("review pendente inválido")
	}
	return name, pending, nil
}

func (h Handler) pendingReview(w http.ResponseWriter, r *http.Request) {
	name, pending, err := h.readPendingReview(r)
	if err != nil {
		if os.IsNotExist(err) || errors.Is(err, reviewlog.ErrNotFound) {
			writeError(w, http.StatusNotFound, "não há review aguardando autorização")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	options := review.BuildPullReviewOptions(pending.FinalReview, h.allowAutonomousReject(r, pending.Job.Owner+"/"+pending.Job.Repo))
	comments := make([]pendingReviewCommentView, 0, len(options.Comments))
	for _, comment := range options.Comments {
		comments = append(comments, pendingReviewCommentView{Path: comment.Path, NewPosition: comment.NewPosition, Body: comment.Body})
	}
	writeJSON(w, http.StatusOK, pendingReviewResponse{
		Name: name, Job: pending.Job, FinalReview: pending.FinalReview,
		Publication: pendingReviewPublicationView{Event: options.Event, Body: options.Body, Comments: comments},
	})
}

func (h Handler) approvePendingReview(w http.ResponseWriter, r *http.Request) {
	name, pending, err := h.readPendingReview(r)
	if err != nil {
		h.pendingError(w, err)
		return
	}
	if h.pendingDecisionExists(name) {
		writeError(w, http.StatusConflict, "esta revisão já foi decidida")
		return
	}
	instanceID, err := h.resolvePendingGiteaInstance(r, pending.Job.GiteaInstanceID, pending.Job.Owner, pending.Job.Repo)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	client, err := h.savedGiteaClient(r, fmt.Sprint(instanceID))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	options := review.BuildPullReviewOptions(pending.FinalReview, h.allowAutonomousReject(r, pending.Job.Owner+"/"+pending.Job.Repo))
	if _, err := client.CreatePullRequestReview(r.Context(), pending.Job.Owner, pending.Job.Repo, pending.Job.PRNumber, options); err != nil {
		writeError(w, http.StatusBadGateway, "não foi possível publicar a revisão no Gitea")
		return
	}
	if err := h.finishPendingReview(name, "approve", "Review autorizado e publicado", true); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"accepted": true, "action": "approve"})
}

func (h Handler) resolvePendingGiteaInstance(r *http.Request, instanceID int64, owner, repo string) (int64, error) {
	if instanceID > 0 {
		return instanceID, nil
	}
	var resolved int64
	err := h.db.QueryRowContext(r.Context(), `SELECT gi.id FROM gitea_instances gi JOIN repositories rep ON rep.gitea_instance_id=gi.id WHERE rep.owner=? AND rep.name=? AND gi.is_enabled=1 ORDER BY gi.id LIMIT 1`, owner, repo).Scan(&resolved)
	if err == sql.ErrNoRows {
		err = h.db.QueryRowContext(r.Context(), `SELECT id FROM gitea_instances WHERE is_enabled=1 ORDER BY is_default DESC, id LIMIT 1`).Scan(&resolved)
	}
	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("nenhuma instância Gitea configurada")
	}
	if err != nil {
		return 0, fmt.Errorf("não foi possível resolver a instância Gitea: %w", err)
	}
	return resolved, nil
}

func (h Handler) decidePendingReview(w http.ResponseWriter, r *http.Request, action string) {
	name, _, err := h.readPendingReview(r)
	if err != nil {
		h.pendingError(w, err)
		return
	}
	if h.pendingDecisionExists(name) {
		writeError(w, http.StatusConflict, "esta revisão já foi decidida")
		return
	}
	if err := h.finishPendingReview(name, action, "Review negado sem publicar", false); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"accepted": true, "action": action})
}

func (h Handler) rerunPendingReview(w http.ResponseWriter, r *http.Request) {
	name, pending, err := h.readPendingReview(r)
	if err != nil {
		h.pendingError(w, err)
		return
	}
	if h.publisher == nil {
		writeError(w, http.StatusServiceUnavailable, "fila de reviews indisponível")
		return
	}
	if h.pendingDecisionExists(name) {
		writeError(w, http.StatusConflict, "esta revisão já foi decidida")
		return
	}
	if h.logStore != nil {
		if err := h.logStore.Delete(r.Context(), name, "pending-review.json"); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	} else {
		pendingPath := filepath.Join(h.logDir, name, "pending-review.json")
		if err := os.Remove(pendingPath); err != nil && !os.IsNotExist(err) {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if err := h.publisher.Publish(r.Context(), pending.Job); err != nil {
		if content, marshalErr := json.Marshal(pending); marshalErr == nil {
			if h.logStore != nil {
				_ = h.logStore.Write(r.Context(), name, "pending-review.json", string(content))
			} else {
				_ = os.WriteFile(filepath.Join(h.logDir, name, "pending-review.json"), content, 0o644)
			}
		}
		writeError(w, http.StatusServiceUnavailable, "não foi possível reexecutar a revisão")
		return
	}
	if err := h.appendProgressEvent(r.Context(), name, pipeline.ProgressEvent{Stage: "pre-publicacao", Status: "done", Percent: 100, Message: "Revisão solicitada novamente ao agente"}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true, "action": "rerun"})
}

func (h Handler) pendingError(w http.ResponseWriter, err error) {
	if os.IsNotExist(err) || errors.Is(err, reviewlog.ErrNotFound) {
		writeError(w, http.StatusNotFound, "não há review aguardando autorização")
		return
	}
	writeError(w, http.StatusBadRequest, err.Error())
}

func (h Handler) pendingDecisionExists(name string) bool {
	if h.logStore != nil {
		exists, _ := h.logStore.Exists(context.Background(), name, "review-decision.json")
		return exists
	}
	_, err := os.Stat(filepath.Join(h.logDir, name, "review-decision.json"))
	return err == nil
}

func (h Handler) finishPendingReview(name, action, message string, published bool) error {
	dir := filepath.Join(h.logDir, name)
	decision, err := json.Marshal(pendingReviewDecision{Action: action, At: time.Now().Format(time.RFC3339)})
	if err != nil {
		return err
	}
	if h.logStore != nil {
		if err := h.logStore.Write(context.Background(), name, "review-decision.json", string(decision)); err != nil {
			return err
		}
	} else if err := os.WriteFile(filepath.Join(dir, "review-decision.json"), decision, 0o644); err != nil {
		return err
	}
	if err := h.appendProgressEvent(context.Background(), name, pipeline.ProgressEvent{Stage: "pre-publicacao", Status: "done", Percent: 100, Message: message}); err != nil {
		return err
	}
	if published {
		return h.appendProgressEvent(context.Background(), name, pipeline.ProgressEvent{Stage: "publicacao", Status: "done", Percent: 100, Message: "Review publicado com autorização manual"})
	}
	return nil
}

func (h Handler) appendProgressEvent(ctx context.Context, name string, event pipeline.ProgressEvent) error {
	if h.logStore != nil {
		if event.Timestamp == "" {
			event.Timestamp = time.Now().Format(time.RFC3339)
		}
		data, err := json.Marshal(event)
		if err != nil {
			return err
		}
		return h.logStore.Append(ctx, name, "00-progress.jsonl", string(data)+"\n")
	}
	return appendProgressEvent(filepath.Join(h.logDir, name), event)
}

func appendProgressEvent(dir string, event pipeline.ProgressEvent) error {
	if event.Timestamp == "" {
		event.Timestamp = time.Now().Format(time.RFC3339)
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(dir, "00-progress.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(append(data, '\n'))
	return err
}

func (h Handler) allowAutonomousReject(r *http.Request, repo string) bool {
	var value int
	_ = h.db.QueryRowContext(r.Context(), `SELECT pol.allow_autonomous_rejection FROM review_profiles rp JOIN review_policies pol ON pol.profile_id=rp.id LEFT JOIN repositories r ON r.review_profile_id=rp.id AND r.full_name=? AND r.is_enabled=1 WHERE rp.is_enabled=1 AND (r.id IS NOT NULL OR rp.is_default=1) ORDER BY r.id DESC, rp.is_default DESC LIMIT 1`, repo).Scan(&value)
	return value != 0
}
