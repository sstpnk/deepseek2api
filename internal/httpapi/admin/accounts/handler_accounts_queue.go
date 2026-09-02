package accounts

import "net/http"

func (h *Handler) queueStatus(w http.ResponseWriter, r *http.Request) {
	if r != nil && r.URL != nil && r.URL.Query().Get("include_accounts") == "1" {
		writeJSON(w, http.StatusOK, h.Pool.Status())
		return
	}
	writeJSON(w, http.StatusOK, h.Pool.StatusSummary())
}
