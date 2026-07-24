package daemon

import (
	"encoding/json"
	"net/http"
)

// ProviderInfo describes an AI provider that Baton can use.
type ProviderInfo struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Configured bool   `json:"configured"`
}

// ConfigureProviderRequest is the JSON body for POST /providers/configure.
type ConfigureProviderRequest struct {
	ProviderID string `json:"provider_id"`
	APIKey     string `json:"api_key"`
}

var knownProviders = []ProviderInfo{
	{ID: "anthropic", Name: "Anthropic"},
	{ID: "deepseek", Name: "DeepSeek"},
	{ID: "opencode_go", Name: "OpenCode Go"},
	{ID: "opencode_zen", Name: "OpenCode Zen"},
}

func (d *Daemon) handleListProviders(w http.ResponseWriter, r *http.Request) {
	providers := make([]ProviderInfo, len(knownProviders))
	for i, p := range knownProviders {
		_, configured := d.config.ProviderKeys[p.ID]
		providers[i] = ProviderInfo{
			ID:         p.ID,
			Name:       p.Name,
			Configured: configured,
		}
	}
	writeJSON(w, http.StatusOK, providers)
}

func (d *Daemon) handleConfigureProvider(w http.ResponseWriter, r *http.Request) {
	var req ConfigureProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	if req.ProviderID == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "provider_id is required")
		return
	}
	if req.APIKey == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "api_key is required")
		return
	}

	// Validate provider ID is known.
	valid := false
	for _, p := range knownProviders {
		if p.ID == req.ProviderID {
			valid = true
			break
		}
	}
	if !valid {
		writeError(w, http.StatusBadRequest, "unknown_provider", "unknown provider ID")
		return
	}

	d.config.ProviderKeys[req.ProviderID] = req.APIKey

	if err := d.config.Save(d.configPath); err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", "failed to save config")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"provider_id": req.ProviderID,
		"configured":  true,
	})
}
