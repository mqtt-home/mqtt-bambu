package web

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/mqtt-home/mqtt-bambu/bambu"
	"github.com/mqtt-home/mqtt-bambu/config"
	"github.com/philipparndt/go-logger"
	loggerchi "github.com/philipparndt/go-logger/chi"
)

type SSEClient struct {
	ID      string
	Channel chan string
}

// WebServer serves the status UI and auth flow. The manager exists from
// startup and gains printers as they are registered (LAN immediately, cloud
// once the account has authenticated).
type WebServer struct {
	cloud   *bambu.Client
	manager *bambu.Manager
	onAuth  func()

	router       *chi.Mux
	sseClients   map[string]*SSEClient
	sseClientsMu sync.RWMutex

	// Liveness: when the bridge first became unhealthy (nil while healthy).
	unhealthySince *time.Time
	unhealthyMu    sync.Mutex
}

func NewWebServer(cloud *bambu.Client, manager *bambu.Manager, onAuth func()) *WebServer {
	ws := &WebServer{
		cloud:      cloud,
		manager:    manager,
		onAuth:     onAuth,
		router:     chi.NewRouter(),
		sseClients: make(map[string]*SSEClient),
	}
	ws.setupRoutes()
	return ws
}

func (ws *WebServer) livenessGrace() time.Duration {
	if s := config.Get().Web.LivenessGraceSeconds; s > 0 {
		return time.Duration(s) * time.Second
	}
	return 4 * time.Minute
}

func (ws *WebServer) setupRoutes() {
	ws.router.Use(loggerchi.LoggerWithLevel(slog.LevelDebug))
	ws.router.Use(middleware.Recoverer)

	ws.router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	ws.router.Route("/api", func(r chi.Router) {
		r.Get("/health", ws.healthCheck)
		r.Get("/livez", ws.liveness)

		r.Get("/auth/status", ws.authStatus)
		r.Post("/auth/request-code", ws.requestCode)
		r.Post("/auth/login", ws.authLogin)
		r.Post("/auth/logout", ws.authLogout)

		r.Get("/devices", ws.getDevices)
		r.Get("/devices/{slug}/status", ws.getDeviceStatus)
		r.Post("/devices/{slug}/refresh", ws.refreshDevice)

		r.Get("/events", ws.handleSSE)
	})

	// SPA fallback.
	distDir := "./web/dist/"
	fileServer := http.FileServer(http.Dir(distDir))
	ws.router.HandleFunc("/*", func(w http.ResponseWriter, r *http.Request) {
		path := "." + r.URL.Path
		if _, err := http.Dir(distDir).Open(path); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, distDir+"index.html")
	})
}

// --- auth ---

func (ws *WebServer) authStatus(w http.ResponseWriter, _ *http.Request) {
	resp := map[string]any{
		"cloud_enabled": config.Get().Bambu.CloudEnabled(),
		"authenticated": ws.cloud.IsAuthenticated(),
		"email":         ws.cloud.Email(),
	}
	if ws.cloud.IsAuthenticated() {
		resp["devices"] = len(ws.cloud.Devices())
	}
	writeJSON(w, resp)
}

func (ws *WebServer) requestCode(w http.ResponseWriter, _ *http.Request) {
	if err := ws.cloud.RequestCode(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ws.jsonOK(w)
}

type codeLoginRequest struct {
	Code string `json:"code"`
}

func (ws *WebServer) authLogin(w http.ResponseWriter, r *http.Request) {
	var req codeLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
		writeError(w, http.StatusBadRequest, "code is required")
		return
	}

	if err := ws.cloud.CodeLogin(req.Code); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := ws.cloud.DiscoverDevices(); err != nil {
		writeError(w, http.StatusInternalServerError, "Login succeeded but device discovery failed: "+err.Error())
		return
	}
	if err := ws.cloud.SaveSession(); err != nil {
		logger.Warn("Failed to save session after login", "error", err)
	}
	if ws.onAuth != nil {
		ws.onAuth()
	}

	writeJSON(w, map[string]any{
		"status":  "success",
		"devices": len(ws.cloud.Devices()),
	})
}

func (ws *WebServer) authLogout(w http.ResponseWriter, _ *http.Request) {
	ws.cloud.ClearSession()
	ws.jsonOK(w)
}

// --- devices ---

func (ws *WebServer) getDevices(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, ws.manager.GetSummaries())
}

func (ws *WebServer) getDeviceStatus(w http.ResponseWriter, r *http.Request) {
	mp := ws.printerFromRequest(w, r)
	if mp == nil {
		return
	}
	status := mp.Client.LastStatus()
	if status == nil {
		writeError(w, http.StatusServiceUnavailable, "no status available")
		return
	}
	writeJSON(w, status)
}

func (ws *WebServer) refreshDevice(w http.ResponseWriter, r *http.Request) {
	mp := ws.printerFromRequest(w, r)
	if mp == nil {
		return
	}
	mp.Client.PushAll()
	ws.jsonOK(w)
}

func (ws *WebServer) printerFromRequest(w http.ResponseWriter, r *http.Request) *bambu.ManagedPrinter {
	mp := ws.manager.GetPrinter(chi.URLParam(r, "slug"))
	if mp == nil {
		writeError(w, http.StatusNotFound, "printer not found")
		return nil
	}
	return mp
}

// --- health / liveness ---

func (ws *WebServer) healthCheck(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{
		"status":        "ok",
		"goroutines":    runtime.NumGoroutine(),
		"authenticated": ws.cloud.IsAuthenticated(),
		"printers":      ws.connectionStates(),
		"timestamp":     time.Now().UTC().Format(time.RFC3339),
	})
}

func (ws *WebServer) connectionStates() map[string]bool {
	return ws.manager.ConnectionStates()
}

// liveness reports on the cloud connection only. A cloud session that stops
// working can be repaired by a restart, so it is worth failing the probe for;
// a LAN printer that is simply powered off is normal and a restart would not
// bring it back, so LAN printers never make the bridge unhealthy.
func (ws *WebServer) liveness(w http.ResponseWriter, _ *http.Request) {
	cloudEnabled := config.Get().Bambu.CloudEnabled()
	authenticated := ws.cloud.IsAuthenticated()
	healthy := !cloudEnabled || (authenticated && ws.manager.ConnectedCountMode(bambu.ModeCloud) > 0)

	now := time.Now()
	grace := ws.livenessGrace()

	ws.unhealthyMu.Lock()
	statusCode, newSince, stuckFor := evaluateLiveness(healthy, ws.unhealthySince, now, grace)
	ws.unhealthySince = newSince
	ws.unhealthyMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]any{
		"healthy":         healthy,
		"cloudEnabled":    cloudEnabled,
		"authenticated":   authenticated,
		"printersOnline":  ws.connectedCount(),
		"stuckForSeconds": int(stuckFor.Seconds()),
		"graceSeconds":    int(grace.Seconds()),
		"timestamp":       now.UTC().Format(time.RFC3339),
	})
}

func (ws *WebServer) connectedCount() int {
	return ws.manager.ConnectedCount()
}

// evaluateLiveness is the pure liveness decision: 503 only once the bridge has
// been continuously unhealthy for longer than the grace window.
func evaluateLiveness(healthy bool, unhealthySince *time.Time, now time.Time, grace time.Duration) (statusCode int, newSince *time.Time, stuckFor time.Duration) {
	if healthy {
		return http.StatusOK, nil, 0
	}
	if unhealthySince == nil {
		t := now
		unhealthySince = &t
	}
	stuckFor = now.Sub(*unhealthySince)
	statusCode = http.StatusOK
	if stuckFor > grace {
		statusCode = http.StatusServiceUnavailable
	}
	return statusCode, unhealthySince, stuckFor
}

// --- SSE ---

// BroadcastStatus sends a per-printer status update to all SSE clients.
func (ws *WebServer) BroadcastStatus(slug string, status bambu.PublishedStatus) {
	payload := struct {
		Type string `json:"type"`
		Slug string `json:"slug"`
		bambu.PublishedStatus
	}{Type: "status", Slug: slug, PublishedStatus: status}
	ws.broadcast(payload)
}

// BroadcastAvailability sends a per-printer online/offline transition.
func (ws *WebServer) BroadcastAvailability(slug string, online bool) {
	payload := struct {
		Type   string `json:"type"`
		Slug   string `json:"slug"`
		Online bool   `json:"online"`
	}{Type: "availability", Slug: slug, Online: online}
	ws.broadcast(payload)
}

func (ws *WebServer) broadcast(payload any) {
	message, err := json.Marshal(payload)
	if err != nil {
		return
	}
	messageStr := string(message)
	ws.sseClientsMu.RLock()
	for _, client := range ws.sseClients {
		select {
		case client.Channel <- messageStr:
		default:
		}
	}
	ws.sseClientsMu.RUnlock()
}

func (ws *WebServer) handleSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	clientID := fmt.Sprintf("%d", time.Now().UnixNano())
	channel := make(chan string, 10)

	ws.sseClientsMu.Lock()
	ws.sseClients[clientID] = &SSEClient{ID: clientID, Channel: channel}
	ws.sseClientsMu.Unlock()

	// Seed the client with current state.
	for _, summary := range ws.manager.GetSummaries() {
		avail := struct {
			Type   string `json:"type"`
			Slug   string `json:"slug"`
			Online bool   `json:"online"`
		}{Type: "availability", Slug: summary.Slug, Online: summary.Online}
		if msg, err := json.Marshal(avail); err == nil {
			fmt.Fprintf(w, "data: %s\n\n", string(msg))
		}
		if summary.Status != nil {
			payload := struct {
				Type string `json:"type"`
				Slug string `json:"slug"`
				bambu.PublishedStatus
			}{Type: "status", Slug: summary.Slug, PublishedStatus: *summary.Status}
			if msg, err := json.Marshal(payload); err == nil {
				fmt.Fprintf(w, "data: %s\n\n", string(msg))
			}
		}
	}

	flusher, ok := w.(http.Flusher)
	if ok {
		flusher.Flush()
	}

	defer func() {
		ws.sseClientsMu.Lock()
		delete(ws.sseClients, clientID)
		close(channel)
		ws.sseClientsMu.Unlock()
	}()

	for {
		select {
		case msg := <-channel:
			if _, err := fmt.Fprintf(w, "data: %s\n\n", msg); err != nil {
				return
			}
			if ok {
				flusher.Flush()
			}
		case <-r.Context().Done():
			return
		}
	}
}

func (ws *WebServer) Start(port int) error {
	addr := ":" + strconv.Itoa(port)
	logger.Info("Starting web server", "address", addr)
	return http.ListenAndServe(addr, ws.router)
}

// --- helpers ---

func (ws *WebServer) jsonOK(w http.ResponseWriter) {
	writeJSON(w, map[string]string{"status": "success"})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
