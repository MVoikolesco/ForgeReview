package admin

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"gitea-agents/internal/http/responses"
	"gitea-agents/internal/integrations/gitea"
	"gitea-agents/internal/queue"
	"gitea-agents/internal/security"

	"github.com/gin-gonic/gin"
)

type giteaConnectionRequest struct {
	BaseURL     string `json:"base_url"`
	Token       string `json:"token"`
	BotUsername string `json:"bot_username"`
}

type giteaRepositoriesRequest struct {
	Organization string `json:"organization"`
	Repositories []struct {
		Owner    string `json:"owner"`
		Name     string `json:"name"`
		FullName string `json:"full_name"`
	} `json:"repositories"`
}

// testGitea validates either supplied Gitea credentials or a saved instance.
func (h *AdminHandler) testGitea(c *gin.Context) {
	var input giteaConnectionRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		responses.LegacyError(c, http.StatusBadRequest, "invalid JSON")
		return
	}

	if input.Token == "" && c.Param("id") != "" {
		client, err := h.savedGitea(c)
		if err != nil {
			responses.LegacyError(c, http.StatusBadRequest, err.Error())
			return
		}
		if err := client.TestConnection(c); err != nil {
			responses.LegacyError(c, http.StatusBadGateway, "falha ao conectar ao Gitea")
			return
		}
		responses.Legacy(c, http.StatusOK, gin.H{"ok": true})
		return
	}

	if strings.TrimSpace(input.BaseURL) == "" || strings.TrimSpace(input.Token) == "" {
		responses.LegacyError(c, http.StatusBadRequest, "base_url e token são obrigatórios")
		return
	}
	if err := gitea.New(input.BaseURL, input.Token).TestConnection(c); err != nil {
		responses.LegacyError(c, http.StatusBadGateway, "falha ao conectar ao Gitea")
		return
	}

	responses.Legacy(c, http.StatusOK, gin.H{
		"ok":           true,
		"bot_username": input.BotUsername,
	})
}

// savedGitea resolves the route ID and returns a client backed by the encrypted
// token stored for that enabled Gitea instance.
func (h *AdminHandler) savedGitea(c *gin.Context) (*gitea.Client, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("instância Gitea inválida")
	}
	return h.savedGiteaByID(c, id)
}

// savedGiteaByID returns a client for an enabled Gitea instance. It returns an
// error when the instance is missing or its token cannot be decrypted.
func (h *AdminHandler) savedGiteaByID(c *gin.Context, id int64) (*gitea.Client, error) {
	var baseURL, ciphertext string
	err := h.db.QueryRowContext(
		c,
		"SELECT base_url,token_ciphertext FROM gitea_instances WHERE id=? AND is_enabled=1",
		id,
	).Scan(&baseURL, &ciphertext)
	if err != nil {
		return nil, err
	}

	token, err := security.Decrypt(ciphertext)
	if err != nil || token == "" {
		return nil, fmt.Errorf("token Gitea indisponível")
	}

	return gitea.New(baseURL, token), nil
}

// resolveGiteaInstance returns the explicit job instance, the instance mapped
// to its repository, or the default enabled instance, in that order.
func (h *AdminHandler) resolveGiteaInstance(c *gin.Context, job queue.ReviewJob) (int64, error) {
	if job.GiteaInstanceID > 0 {
		return job.GiteaInstanceID, nil
	}

	var id int64
	err := h.db.QueryRowContext(
		c,
		`SELECT gi.id
		 FROM gitea_instances gi
		 JOIN repositories rep ON rep.gitea_instance_id=gi.id
		 WHERE rep.owner=? AND rep.name=? AND gi.is_enabled=1
		 ORDER BY rep.id LIMIT 1`,
		job.Owner,
		job.Repository,
	).Scan(&id)
	if err == sql.ErrNoRows {
		err = h.db.QueryRowContext(
			c,
			`SELECT id FROM gitea_instances
			 WHERE is_enabled=1
			 ORDER BY is_default DESC, id LIMIT 1`,
		).Scan(&id)
	}
	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("nenhuma instância Gitea configurada")
	}

	return id, err
}

// organizations lists the organizations visible to a saved Gitea instance.
func (h *AdminHandler) organizations(c *gin.Context) {
	client, err := h.savedGitea(c)
	if err != nil {
		responses.LegacyError(c, http.StatusBadRequest, err.Error())
		return
	}

	items, err := client.ListOrganizations(c)
	if err != nil {
		responses.LegacyError(c, http.StatusBadGateway, "falha ao listar organizações")
		return
	}

	responses.Legacy(c, http.StatusOK, items)
}

// giteaRepositories either lists repositories from Gitea or replaces the
// persisted repository selection when the request contains repositories.
func (h *AdminHandler) giteaRepositories(c *gin.Context) {
	client, err := h.savedGitea(c)
	if err != nil {
		responses.LegacyError(c, http.StatusBadRequest, err.Error())
		return
	}

	var body giteaRepositoriesRequest
	_ = c.ShouldBindJSON(&body)
	if body.Repositories != nil {
		h.saveGiteaRepositories(c, body)
		return
	}

	items, err := client.ListRepositories(c, body.Organization)
	if err != nil {
		responses.LegacyError(c, http.StatusBadGateway, "falha ao listar repositórios")
		return
	}

	responses.Legacy(c, http.StatusOK, items)
}

// saveGiteaRepositories atomically replaces the repository selection for the
// Gitea instance identified by the current route.
func (h *AdminHandler) saveGiteaRepositories(c *gin.Context, body giteaRepositoriesRequest) {
	tx, err := h.db.BeginTx(c, nil)
	if err == nil {
		_, err = tx.ExecContext(c, "DELETE FROM repositories WHERE gitea_instance_id=?", c.Param("id"))
		for _, item := range body.Repositories {
			if err != nil {
				break
			}

			fullName := item.FullName
			if fullName == "" {
				fullName = item.Owner + "/" + item.Name
			}
			_, err = tx.ExecContext(
				c,
				"INSERT INTO repositories(gitea_instance_id,owner,name,full_name) VALUES(?,?,?,?)",
				c.Param("id"),
				item.Owner,
				item.Name,
				fullName,
			)
		}

		if err == nil {
			err = tx.Commit()
		} else {
			_ = tx.Rollback()
		}
	}
	if err != nil {
		responses.LegacyError(c, http.StatusBadRequest, "could not save repositories")
		return
	}

	responses.Legacy(c, http.StatusOK, gin.H{"saved": len(body.Repositories)})
}

// pullRequests lists open pull requests for a repository visible to the saved
// Gitea instance.
func (h *AdminHandler) pullRequests(c *gin.Context) {
	client, err := h.savedGitea(c)
	if err != nil {
		responses.LegacyError(c, http.StatusBadRequest, err.Error())
		return
	}

	var body struct {
		Owner string `json:"owner"`
		Repo  string `json:"repo"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Owner == "" || body.Repo == "" {
		responses.LegacyError(c, http.StatusBadRequest, "owner e repo são obrigatórios")
		return
	}

	items, err := client.ListPullRequests(c, body.Owner, body.Repo)
	if err != nil {
		responses.LegacyError(c, http.StatusBadGateway, "falha ao listar pull requests")
		return
	}

	responses.Legacy(c, http.StatusOK, items)
}
