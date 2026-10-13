package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"lotse.sh/internal/protocol"
)

// state is what the agent keeps in its data directory: its credential,
// from enrolling on. The enrollment token is not kept.
type state struct {
	Credential string `json:"credential"`
	HostID     int64  `json:"host_id"`
	HostName   string `json:"host_name"`
}

const stateFile = "agent.json"

func (a *agent) statePath() string { return filepath.Join(a.cfg.DataDir, stateFile) }

func (a *agent) loadState() (state, error) {
	var s state
	b, err := os.ReadFile(a.statePath())
	if err != nil {
		return s, err
	}
	return s, json.Unmarshal(b, &s)
}

// saveState writes the state whole and then renames it; only the agent may
// read it.
func (a *agent) saveState(s state) error {
	if err := os.MkdirAll(a.cfg.DataDir, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := a.statePath() + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, a.statePath())
}

var errRevoked = errors.New("the hub does not accept this agent's credential any more")

// credential returns the agent's credential, enrolling first if it has
// none: then the token from the install command is needed, once.
func (a *agent) credential(ctx context.Context) (string, error) {
	s, err := a.loadState()
	if err == nil && s.Credential != "" {
		return s.Credential, nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("reading %s: %w", a.statePath(), err)
	}
	if a.cfg.Token == "" {
		return "", fmt.Errorf("not enrolled yet, and no enrollment token given")
	}
	hello := a.host.hello(ctx)
	body, _ := json.Marshal(protocol.EnrollRequest{
		Token: a.cfg.Token, Hostname: hello.Hostname, MachineID: protocol.MachineKey(hello.MachineID, hello.Hostname),
	})
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimSuffix(a.cfg.Hub, "/")+protocol.EnrollPath, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized {
		return "", errors.New("the hub does not accept this enrollment token; take the current one from the hub")
	}
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("enrolling: %s", res.Status)
	}
	var e protocol.EnrollResponse
	if err := json.NewDecoder(res.Body).Decode(&e); err != nil {
		return "", err
	}
	if err := a.saveState(state{Credential: e.Credential, HostID: e.HostID, HostName: e.HostName}); err != nil {
		return "", err
	}
	a.log.Info("enrolled", "host", e.HostName, "environment", e.Environment)
	return e.Credential, nil
}

// checkCredential asks the hub whether credential is still accepted.
func (a *agent) checkCredential(ctx context.Context, credential string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(a.cfg.Hub, "/")+protocol.CheckPath, nil)
	req.Header.Set("Authorization", "Bearer "+credential)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	res.Body.Close()
	switch res.StatusCode {
	case http.StatusNoContent:
		return true, nil
	case http.StatusUnauthorized:
		return false, nil
	}
	return false, fmt.Errorf("checking the credential: %s", res.Status)
}
