package hub

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"

	"lotse.sh/internal/version"
)

type health struct {
	Status   string `json:"status"`
	Version  string `json:"version"`
	Database string `json:"database"`
	// Setup is true while the hub waits to be set up.
	Setup bool `json:"setup"`
	// Agents is how many agents are connected.
	Agents int `json:"agents"`
}

func (h *hub) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, health{
		Status:   "ok",
		Version:  version.Version,
		Database: h.db.Dialect().String(),
		Setup:    h.setupToken != "",
		Agents:   h.agents.count(),
	})
}

// checkSetup says whether a setup token is the current one, so the UI can
// tell a stale link from a good one before asking for anything.
func (h *hub) checkSetup(w http.ResponseWriter, r *http.Request) {
	if h.setupToken == "" {
		writeError(w, http.StatusConflict, "this hub is already set up")
		return
	}
	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "the request is not valid JSON")
		return
	}
	valid := subtle.ConstantTimeCompare([]byte(req.Token), []byte(h.setupToken)) == 1
	writeJSON(w, http.StatusOK, map[string]bool{"valid": valid})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
