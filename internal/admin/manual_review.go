package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"gitea-agents/internal/queue"
)

func (h Handler) manualReview(w http.ResponseWriter, r *http.Request) {
	var input struct {
		InstanceID int64  `json:"instance_id"`
		Owner      string `json:"owner"`
		Repo       string `json:"repo"`
		PRNumber   int    `json:"pr_number"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.InstanceID <= 0 || strings.TrimSpace(input.Owner) == "" || strings.TrimSpace(input.Repo) == "" || input.PRNumber <= 0 {
		writeError(w, 400, "instância, owner, repositório e PR são obrigatórios")
		return
	}
	if h.publisher == nil {
		writeError(w, 503, "fila de reviews indisponível")
		return
	}
	client, err := h.savedGiteaClient(r, fmt.Sprint(input.InstanceID))
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	pr, err := client.GetPullRequest(r.Context(), input.Owner, input.Repo, input.PRNumber)
	if err != nil {
		writeError(w, 502, "não foi possível validar o pull request")
		return
	}
	if pr.Number != input.PRNumber {
		writeError(w, http.StatusBadGateway, "o Gitea retornou um pull request diferente do solicitado")
		return
	}
	if pr.State != "open" {
		writeError(w, 409, "o pull request não está aberto")
		return
	}
	job := queue.ReviewJob{GiteaInstanceID: input.InstanceID, Owner: input.Owner, Repo: input.Repo, PRNumber: pr.Number, Manual: true, Title: pr.Title, Description: pr.Body, Author: pr.User.Login, BaseBranch: pr.Base.Ref, HeadBranch: pr.Head.Ref}
	if err := h.publisher.Publish(r.Context(), job); err != nil {
		writeError(w, 503, "não foi possível enfileirar a revisão")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true, "instance_id": job.GiteaInstanceID, "owner": job.Owner, "repo": job.Repo, "pr_number": job.PRNumber, "title": job.Title})
}
