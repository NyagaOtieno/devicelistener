package httpapi

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"gps-listener-backend/internal/storage"
)

type Server struct{ store *storage.Store }

func New(store *storage.Store) *Server { return &Server{store: store} }

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.health)
	mux.HandleFunc("/devices", s.devices)
	mux.HandleFunc("/devices/", s.deviceRoutes)
	mux.HandleFunc("/commands", s.commands)
	return loggingMiddleware(cors(mux))
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) devices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	items, err := s.store.ListDevices(ctx)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, items)
}

func (s *Server) deviceRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/devices/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	imei := parts[0]
	if len(parts) == 1 && r.Method == http.MethodGet {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		latest, err := s.store.GetLatestTelemetry(ctx, imei)
		if err != nil {
			writeError(w, 500, err)
			return
		}
		writeJSON(w, 200, latest)
		return
	}
	if len(parts) == 2 && parts[1] == "commands" && r.Method == http.MethodPost {
		var body struct {
			Protocol string `json:"protocol"`
			Command  string `json:"command"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, 400, err)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		protocol := body.Protocol
		if protocol == "" {
			device, err := s.store.GetDevice(ctx, imei)
			if err == nil {
				protocol = device.Protocol
			}
		}
		item, err := s.store.QueueCommand(ctx, imei, protocol, body.Command)
		if err != nil {
			writeError(w, 500, err)
			return
		}
		writeJSON(w, 201, item)
		return
	}
	http.NotFound(w, r)
}

func (s *Server) commands(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	items, err := s.store.ListCommands(ctx, r.URL.Query().Get("imei"), r.URL.Query().Get("status"), 100)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, items)
}

func methodNotAllowed(w http.ResponseWriter) {
	writeJSON(w, 405, map[string]string{"error": "method not allowed"})
}
func writeError(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("api %s %s ua=%s", r.Method, r.URL.Path, r.UserAgent())
		next.ServeHTTP(w, r)
	})
}
func ListenAddr() string {
	if v := os.Getenv("API_ADDR"); v != "" {
		return v
	}
	return ":8080"
}
