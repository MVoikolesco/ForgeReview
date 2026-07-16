package admin

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"gitea-agents/internal/gitea"
)

type giteaInput struct {
	Name        string `json:"name"`
	BaseURL     string `json:"base_url"`
	BotUsername string `json:"bot_username"`
	Token       string `json:"token"`
}
type repoSelection struct {
	Repositories []struct {
		Owner    string `json:"owner"`
		Name     string `json:"name"`
		FullName string `json:"full_name"`
	} `json:"repositories"`
}

func validGiteaInput(v giteaInput, tokenRequired bool) error {
	parsed, err := url.Parse(strings.TrimSpace(v.BaseURL))
	if err != nil || parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" {
		return fmt.Errorf("URL Gitea inválida")
	}
	if strings.TrimSpace(v.Name) == "" || len(v.Name) > 120 {
		return fmt.Errorf("nome é obrigatório")
	}
	if strings.TrimSpace(v.BotUsername) == "" {
		return fmt.Errorf("usuário bot é obrigatório")
	}
	if tokenRequired && strings.TrimSpace(v.Token) == "" {
		return fmt.Errorf("token é obrigatório")
	}
	return nil
}

func (h Handler) giteaRoute(w http.ResponseWriter, r *http.Request, suffix string) bool {
	parts := strings.Split(strings.Trim(suffix, "/"), "/")
	if len(parts) == 1 && parts[0] == "test" && r.Method == http.MethodPost {
		h.giteaTest(w, r)
		return true
	}
	if len(parts) == 1 && parts[0] == "" && r.Method == http.MethodPost {
		h.giteaCreate(w, r)
		return true
	}
	if len(parts) == 1 && r.Method == http.MethodDelete {
		h.giteaDelete(w, r, parts[0])
		return true
	}
	if len(parts) == 1 && (r.Method == http.MethodPatch || r.Method == http.MethodPut) {
		h.giteaUpdate(w, r, parts[0])
		return true
	}
	if len(parts) < 2 {
		return false
	}
	id := parts[0]
	action := parts[1]
	if action == "test" && r.Method == http.MethodPost {
		h.giteaSavedTest(w, r, id)
		return true
	}
	if action == "organizations" && r.Method == http.MethodPost {
		h.giteaOrganizations(w, r, id)
		return true
	}
	if action == "repositories" && r.Method == http.MethodPost {
		h.giteaRepositories(w, r, id)
		return true
	}
	return false
}

func decodeGitea(r *http.Request) (giteaInput, error) {
	var v giteaInput
	if err := json.NewDecoder(io.LimitReader(r.Body, 32<<10)).Decode(&v); err != nil {
		return v, fmt.Errorf("JSON inválido")
	}
	return v, nil
}
func (h Handler) giteaCreate(w http.ResponseWriter, r *http.Request) {
	var count int
	if err := h.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM gitea_instances").Scan(&count); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	if count > 0 {
		writeError(w, http.StatusConflict, "remova a conexão atual antes de cadastrar outra")
		return
	}
	v, err := decodeGitea(r)
	if err == nil {
		err = validGiteaInput(v, true)
	}
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	id, err := h.saveGitea(r, 0, v)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	h.respondOne(w, r, "gitea_instances", id, 201)
}
func (h Handler) giteaTest(w http.ResponseWriter, r *http.Request) {
	v, err := decodeGitea(r)
	if err == nil {
		err = validGiteaInput(v, true)
	}
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	h.testGiteaClient(w, r, v.BaseURL, v.Token, v.BotUsername)
}
func (h Handler) giteaSavedTest(w http.ResponseWriter, r *http.Request, id string) {
	var v giteaInput
	var base, user, cipherText, env string
	err := h.db.QueryRowContext(r.Context(), "SELECT base_url,bot_username,token_ciphertext,token_env_name FROM gitea_instances WHERE id=?", id).Scan(&base, &user, &cipherText, &env)
	if err != nil {
		writeError(w, 404, "organização não encontrada")
		return
	}
	v, err = decodeGitea(r)
	if err != nil {
		writeError(w, 400, "JSON inválido")
		return
	}
	if v.BaseURL == "" {
		v.BaseURL = base
	}
	if v.BotUsername == "" {
		v.BotUsername = user
	}
	if v.Token == "" {
		v.Token, err = h.decryptToken(cipherText)
		if err != nil {
			v.Token = os.Getenv(env)
		}
	}
	if err != nil || v.Token == "" {
		writeError(w, 400, "token não disponível")
		return
	}
	h.testGiteaClient(w, r, v.BaseURL, v.Token, v.BotUsername)
}

func (h Handler) giteaDelete(w http.ResponseWriter, r *http.Request, id string) {
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(r.Context(), "DELETE FROM repositories WHERE gitea_instance_id=?", id); err == nil {
		var result sql.Result
		result, err = tx.ExecContext(r.Context(), "DELETE FROM gitea_instances WHERE id=?", id)
		if err == nil {
			var affected int64
			affected, _ = result.RowsAffected()
			if affected == 0 {
				writeError(w, 404, "not found")
				return
			}
		}
	}
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if err = tx.Commit(); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h Handler) testGiteaClient(w http.ResponseWriter, r *http.Request, base, token, user string) {
	if err := gitea.NewClient(base, token).TestConnection(r.Context()); err != nil {
		writeError(w, 502, "falha ao conectar ao Gitea")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "bot_username": user})
}
func (h Handler) giteaOrganizations(w http.ResponseWriter, r *http.Request, id string) {
	c, err := h.savedGiteaClient(r, id)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	result, err := c.ListOrganizations(r.Context())
	if err != nil {
		writeError(w, 502, "falha ao listar organizações")
		return
	}
	writeJSON(w, 200, result)
}
func (h Handler) giteaRepositories(w http.ResponseWriter, r *http.Request, id string) {
	c, err := h.savedGiteaClient(r, id)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	var body struct {
		Organization string `json:"organization"`
		Repositories []struct {
			Owner    string `json:"owner"`
			Name     string `json:"name"`
			FullName string `json:"full_name"`
		} `json:"repositories"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Repositories != nil {
		if err := h.saveRepositories(r, id, body.Repositories); err != nil {
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"saved": len(body.Repositories)})
		return
	}
	result, err := c.ListRepositories(r.Context(), body.Organization)
	if err != nil {
		writeError(w, 502, "falha ao listar repositórios")
		return
	}
	writeJSON(w, 200, result)
}
func (h Handler) saveRepositories(r *http.Request, id string, repos []struct {
	Owner    string `json:"owner"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
}) error {
	var instance int64
	if err := h.db.QueryRowContext(r.Context(), "SELECT id FROM gitea_instances WHERE id=?", id).Scan(&instance); err != nil {
		return fmt.Errorf("organização não encontrada")
	}
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(r.Context(), "DELETE FROM repositories WHERE gitea_instance_id=?", instance); err != nil {
		return err
	}
	for _, repo := range repos {
		full := repo.FullName
		if full == "" {
			full = repo.Owner + "/" + repo.Name
		}
		if _, err = tx.ExecContext(r.Context(), "INSERT INTO repositories(gitea_instance_id,owner,name,full_name) VALUES(?,?,?,?)", instance, repo.Owner, repo.Name, full); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (h Handler) giteaUpdate(w http.ResponseWriter, r *http.Request, id string) {
	var current giteaInput
	var old, env string
	if err := h.db.QueryRowContext(r.Context(), "SELECT name,base_url,bot_username,token_ciphertext,token_env_name FROM gitea_instances WHERE id=?", id).Scan(&current.Name, &current.BaseURL, &current.BotUsername, &old, &env); err != nil {
		writeError(w, 404, "organização não encontrada")
		return
	}
	incoming, err := decodeGitea(r)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if incoming.Name != "" {
		current.Name = incoming.Name
	}
	if incoming.BaseURL != "" {
		current.BaseURL = incoming.BaseURL
	}
	if incoming.BotUsername != "" {
		current.BotUsername = incoming.BotUsername
	}
	if incoming.Token != "" {
		current.Token = incoming.Token
	} else {
		current.Token, err = h.decryptToken(old)
		if err != nil {
			current.Token = os.Getenv(env)
		}
	}
	if err = validGiteaInput(current, false); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if current.Token == "" {
		writeError(w, 400, "token não disponível")
		return
	}
	numeric, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		writeError(w, 400, "id inválido")
		return
	}
	if _, err = h.saveGitea(r, numeric, current); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	h.respondOne(w, r, "gitea_instances", numeric, 200)
}
func (h Handler) savedGiteaClient(r *http.Request, id string) (*gitea.Client, error) {
	var base, user, ct, env string
	if err := h.db.QueryRowContext(r.Context(), "SELECT base_url,bot_username,token_ciphertext,token_env_name FROM gitea_instances WHERE id=?", id).Scan(&base, &user, &ct, &env); err != nil {
		return nil, fmt.Errorf("organização não encontrada")
	}
	token, err := h.decryptToken(ct)
	if err != nil {
		token = os.Getenv(env)
	}
	if token == "" {
		return nil, fmt.Errorf("token não disponível")
	}
	return gitea.NewClient(base, token), nil
}
func (h Handler) saveGitea(r *http.Request, id int64, v giteaInput) (int64, error) {
	ct, err := h.encryptToken(v.Token)
	if err != nil {
		return 0, err
	}
	if id == 0 {
		res, err := h.db.ExecContext(r.Context(), "INSERT INTO gitea_instances(name,base_url,bot_username,token_ciphertext) VALUES(?,?,?,?)", v.Name, v.BaseURL, v.BotUsername, ct)
		if err != nil {
			return 0, err
		}
		return res.LastInsertId()
	}
	_, err = h.db.ExecContext(r.Context(), "UPDATE gitea_instances SET name=?,base_url=?,bot_username=?,token_ciphertext=?,updated_at=CURRENT_TIMESTAMP WHERE id=?", v.Name, v.BaseURL, v.BotUsername, ct, id)
	return id, err
}
func (h Handler) encryptToken(token string) (string, error) {
	key := []byte(os.Getenv("GITEA_TOKEN_ENCRYPTION_KEY"))
	if len(key) != 32 {
		return "", fmt.Errorf("GITEA_TOKEN_ENCRYPTION_KEY deve ter 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	g, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, g.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(g.Seal(nonce, nonce, []byte(token), nil)), nil
}
func (h Handler) decryptToken(value string) (string, error) {
	key := []byte(os.Getenv("GITEA_TOKEN_ENCRYPTION_KEY"))
	if len(key) != 32 {
		return "", fmt.Errorf("chave de criptografia inválida")
	}
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	g, err := cipher.NewGCM(block)
	if err != nil || len(raw) < g.NonceSize() {
		return "", fmt.Errorf("token inválido")
	}
	out, err := g.Open(nil, raw[:g.NonceSize()], raw[g.NonceSize():], nil)
	return string(out), err
}
