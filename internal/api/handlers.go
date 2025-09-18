package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"codex-backup-tool/internal/core"
)

// API 聚合 HTTP 处理逻辑。
type API struct {
	svc *core.Service
}

// New 构造 API。
func New(svc *core.Service) *API {
	return &API{svc: svc}
}

// Register 将 API 注册到 mux。
func (a *API) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/status", a.handleStatus)
	mux.HandleFunc("/api/scan", a.handleScan)
	mux.HandleFunc("/api/backups", a.handleBackupsRoot)
	mux.HandleFunc("/api/backups/", a.handleBackupByID)
	mux.HandleFunc("/api/codex/login", a.handleCodexLogin)
}

func (a *API) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		notAllowed(w, http.MethodGet)
		return
	}
	status, err := a.svc.Status()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeOK(w, status)
}

func (a *API) handleScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		notAllowed(w, http.MethodPost)
		return
	}
	var req struct {
		Remark *string `json:"remark"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	res, err := a.svc.Scan(false, req.Remark)
	if err != nil {
		status, msg := mapServiceError(err)
		writeErrorWithMessage(w, status, msg)
		return
	}
	writeOK(w, res)
}

func (a *API) handleBackupsRoot(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := a.svc.ListBackups()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeOK(w, items)
	case http.MethodPost:
		var req struct {
			Remark *string `json:"remark"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		res, err := a.svc.CreateBackup(req.Remark)
		if err != nil {
			status, msg := mapServiceError(err)
			writeErrorWithMessage(w, status, msg)
			return
		}
		writeOK(w, res)
	default:
		notAllowed(w, http.MethodGet, http.MethodPost)
	}
}

func (a *API) handleBackupByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/backups/")
	if rest == "" {
		writeErrorWithMessage(w, http.StatusBadRequest, "缺少备份 ID")
		return
	}
	parts := strings.Split(rest, "/")
	id := parts[0]
	if id == "" {
		writeErrorWithMessage(w, http.StatusBadRequest, "无效的备份 ID")
		return
	}
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodDelete:
			if err := a.svc.DeleteBackup(id); err != nil {
				status, msg := mapServiceError(err)
				writeErrorWithMessage(w, status, msg)
				return
			}
			writeOK(w, map[string]string{"deleted": id})
		default:
			notAllowed(w, http.MethodDelete)
		}
		return
	}
	action := parts[1]
	switch action {
	case "remark":
		if r.Method != http.MethodPatch {
			notAllowed(w, http.MethodPatch)
			return
		}
		var req struct {
			Remark string `json:"remark"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		item, err := a.svc.UpdateRemark(id, req.Remark)
		if err != nil {
			status, msg := mapServiceError(err)
			writeErrorWithMessage(w, status, msg)
			return
		}
		writeOK(w, item)
	case "restore":
		if r.Method != http.MethodPost {
			notAllowed(w, http.MethodPost)
			return
		}
		if err := a.svc.RestoreBackup(id); err != nil {
			status, msg := mapServiceError(err)
			writeErrorWithMessage(w, status, msg)
			return
		}
		writeOK(w, map[string]string{"restored": id})
	default:
		writeErrorWithMessage(w, http.StatusNotFound, "未知操作")
	}
}

func (a *API) handleCodexLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		notAllowed(w, http.MethodPost)
		return
	}
	stdout, stderr, exitCode, err := a.svc.CodexLogin(r.Context())
	payload := map[string]interface{}{"stdout": stdout, "stderr": stderr, "exit_code": exitCode}
	if err != nil {
		writeJSON(w, http.StatusOK, response{Ok: false, Error: err.Error(), Data: payload})
		return
	}
	writeOK(w, payload)
}

// ---- 辅助函数 ----

type response struct {
	Ok    bool        `json:"ok"`
	Data  interface{} `json:"data,omitempty"`
	Error string      `json:"error,omitempty"`
}

func writeOK(w http.ResponseWriter, data interface{}) {
	writeJSON(w, http.StatusOK, response{Ok: true, Data: data})
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeErrorWithMessage(w, status, err.Error())
}

func writeErrorWithMessage(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, response{Ok: false, Error: msg})
}

func writeJSON(w http.ResponseWriter, status int, resp response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(resp)
}

func notAllowed(w http.ResponseWriter, methods ...string) {
	w.Header().Set("Allow", strings.Join(methods, ", "))
	writeErrorWithMessage(w, http.StatusMethodNotAllowed, "Method Not Allowed")
}

func decodeJSON(r *http.Request, v interface{}) error {
	if r.Body == nil {
		return nil
	}
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, v); err != nil {
		return err
	}
	return nil
}

func mapServiceError(err error) (int, string) {
	switch {
	case errors.Is(err, core.ErrRemarkExists):
		return http.StatusConflict, "备注已存在"
	case errors.Is(err, core.ErrBackupNotFound):
		return http.StatusNotFound, "备份不存在"
	default:
		return http.StatusInternalServerError, err.Error()
	}
}
