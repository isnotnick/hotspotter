package main

import (
	"encoding/json"
	"net/http"
)

// Handlers holds dependencies for HTTP handlers.
type Handlers struct {
	hotspot *HotspotManager
	wifi    *WifiManager
	config  *ConfigStore
}

// NewHandlers creates a new Handlers instance.
func NewHandlers(h *HotspotManager, w *WifiManager, c *ConfigStore) *Handlers {
	return &Handlers{hotspot: h, wifi: w, config: c}
}

// RegisterRoutes sets up all API routes on the given mux.
func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/hotspot/start", h.hotspotStart)
	mux.HandleFunc("/api/hotspot/stop", h.hotspotStop)
	mux.HandleFunc("/api/hotspot/status", h.hotspotStatus)
	mux.HandleFunc("/api/wifi/scan", h.wifiScan)
	mux.HandleFunc("/api/wifi/connect", h.wifiConnect)
	mux.HandleFunc("/api/wifi/disconnect", h.wifiDisconnect)
	mux.HandleFunc("/api/wifi/status", h.wifiStatus)
	mux.HandleFunc("/api/wifi/saved", h.savedNetworks)
	mux.HandleFunc("/api/wifi/saved/remove", h.removeSavedNetwork)
	mux.HandleFunc("/api/interfaces", h.listInterfaces)
	mux.HandleFunc("/api/config/hotspot", h.getHotspotConfig)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (h *Handlers) hotspotStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	var cfg HotspotConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if err := h.hotspot.Start(cfg); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Persist hotspot config for daemon restarts.
	_ = h.config.SaveHotspotConfig(&cfg)

	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

func (h *Handlers) hotspotStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	if err := h.hotspot.Stop(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func (h *Handlers) hotspotStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	status := h.hotspot.Status()
	writeJSON(w, http.StatusOK, status)
}

func (h *Handlers) wifiScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}

	iface := r.URL.Query().Get("iface")
	networks, err := h.wifi.ScanNetworks(iface)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if networks == nil {
		networks = []WifiNetwork{}
	}

	// Mark networks that are saved.
	saved := h.config.GetSavedNetworks()
	savedMap := make(map[string]bool)
	for _, s := range saved {
		savedMap[s.SSID] = true
	}
	for i := range networks {
		networks[i].Saved = savedMap[networks[i].SSID]
	}

	writeJSON(w, http.StatusOK, networks)
}

func (h *Handlers) wifiConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	var req struct {
		Interface string `json:"interface"`
		SSID      string `json:"ssid"`
		Password  string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.SSID == "" {
		writeError(w, http.StatusBadRequest, "ssid is required")
		return
	}

	if err := h.wifi.Connect(req.Interface, req.SSID, req.Password); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Remember this network for auto-reconnect.
	_ = h.config.SaveNetwork(req.SSID, req.Password)

	writeJSON(w, http.StatusOK, map[string]string{"status": "connected", "ssid": req.SSID})
}

func (h *Handlers) wifiDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	var req struct {
		Interface string `json:"interface"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	if err := h.wifi.Disconnect(req.Interface); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "disconnected"})
}

func (h *Handlers) wifiStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}

	iface := r.URL.Query().Get("iface")
	ssid, err := h.wifi.ConnectionStatus(iface)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	connected := ssid != ""
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"connected": connected,
		"ssid":      ssid,
	})
}

func (h *Handlers) savedNetworks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	networks := h.config.GetSavedNetworks()
	if networks == nil {
		networks = []SavedNetwork{}
	}
	writeJSON(w, http.StatusOK, networks)
}

func (h *Handlers) removeSavedNetwork(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req struct {
		SSID string `json:"ssid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if err := h.config.RemoveNetwork(req.SSID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

func (h *Handlers) getHotspotConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	cfg := h.config.GetHotspotConfig()
	if cfg == nil {
		cfg = &HotspotConfig{}
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (h *Handlers) listInterfaces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}

	ifaces, err := h.wifi.ListInterfaces()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if ifaces == nil {
		ifaces = []NetworkInterface{}
	}
	writeJSON(w, http.StatusOK, ifaces)
}
