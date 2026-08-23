package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/ThyFriendlyFox/snapshot-contain-protect/internal/store"
)

type snapshotRequest struct {
	Workset string `json:"workset"`
	Label   string `json:"label"`
	Auto    bool   `json:"auto"`
}

type snapshotResponse struct {
	ID        string `json:"id"`
	CreatedAt int64  `json:"created_at"`
	DurationM int64  `json:"duration_ms"`
}

type restoreRequest struct {
	ID      string `json:"id"`
	Confirm bool   `json:"confirm"`
}

type worksetRequest struct {
	Name      string   `json:"name"`
	Paths     []string `json:"paths"`
	Container bool     `json:"container"`
}

type pruneResponse struct {
	Removed []string `json:"removed"`
}

// restoreResponse is the new snapshot node, plus what the safety snapshot
// before it managed to cover. The node's own fields stay at the top level.
type restoreResponse struct {
	store.Snapshot
	SafetySnapshot *string `json:"safety_snapshot"`
	SafetyWarning  string  `json:"safety_warning,omitempty"`
}

// Handler returns the router. Every route is one of the five verbs, or the
// workset declaration the verbs need.
func (s *Service) Handler(log *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /snapshot", s.handleCreateSnapshot)
	mux.HandleFunc("GET /snapshots", s.handleListSnapshots)
	mux.HandleFunc("GET /diff", s.handleDiff)
	mux.HandleFunc("POST /restore", s.handleRestore)
	mux.HandleFunc("DELETE /snapshots/{id}", s.handlePrune)

	mux.HandleFunc("POST /worksets", s.handleCreateWorkset)
	mux.HandleFunc("GET /worksets", s.handleListWorksets)
	mux.HandleFunc("GET /healthz", s.handleHealth)

	return logging(log, mux)
}

func (s *Service) handleCreateSnapshot(w http.ResponseWriter, r *http.Request) {
	var req snapshotRequest
	if !decode(w, r, &req) {
		return
	}
	sn, took, err := s.CreateSnapshot(r.Context(), req.Workset, req.Label, req.Auto)
	if err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusCreated, snapshotResponse{
		ID:        sn.ID,
		CreatedAt: sn.CreatedAt,
		DurationM: took.Milliseconds(),
	})
}

func (s *Service) handleListSnapshots(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("workset")
	if name == "" {
		fail(w, badRequest("query needs workset=<name>"))
		return
	}
	list, err := s.ListSnapshots(r.Context(), name)
	if err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusOK, list)
}

func (s *Service) handleDiff(w http.ResponseWriter, r *http.Request) {
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	if from == "" || to == "" {
		fail(w, badRequest("query needs from=<id> and to=<id>"))
		return
	}
	change, err := s.Diff(r.Context(), from, to)
	if err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusOK, change)
}

func (s *Service) handleRestore(w http.ResponseWriter, r *http.Request) {
	var req restoreRequest
	if !decode(w, r, &req) {
		return
	}
	if req.ID == "" {
		fail(w, badRequest("restore needs id"))
		return
	}
	if !req.Confirm {
		// The confirm flag stops an agent from rolling back by accident.
		fail(w, badRequest("restore needs confirm=true"))
		return
	}
	result, err := s.Restore(r.Context(), req.ID)
	if err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusCreated, restoreResponse{
		Snapshot:       result.Node,
		SafetySnapshot: result.SafetyID,
		SafetyWarning:  result.Warning,
	})
}

func (s *Service) handlePrune(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	cascade := r.URL.Query().Get("cascade") == "true"
	removed, err := s.Prune(r.Context(), id, cascade)
	if err != nil {
		fail(w, err)
		return
	}
	ids := make([]string, len(removed))
	for i, sn := range removed {
		ids[i] = sn.ID
	}
	write(w, http.StatusOK, pruneResponse{Removed: ids})
}

func (s *Service) handleCreateWorkset(w http.ResponseWriter, r *http.Request) {
	var req worksetRequest
	if !decode(w, r, &req) {
		return
	}
	ws, err := s.CreateWorkset(r.Context(), req.Name, req.Paths, req.Container)
	if err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusCreated, ws)
}

func (s *Service) handleListWorksets(w http.ResponseWriter, r *http.Request) {
	list, err := s.ListWorksets(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	write(w, http.StatusOK, list)
}

func (s *Service) handleHealth(w http.ResponseWriter, r *http.Request) {
	write(w, http.StatusOK, map[string]string{"status": "ok", "backend": s.Backend()})
}

func decode(w http.ResponseWriter, r *http.Request, into any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		fail(w, badRequest("body is not valid JSON for this verb: "+err.Error()))
		return false
	}
	return true
}

func write(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("write response", "error", err)
	}
}

func fail(w http.ResponseWriter, err error) {
	var ae apiError
	status := http.StatusInternalServerError
	if errors.As(err, &ae) {
		status = ae.status
	}
	if errors.Is(err, store.ErrNotFound) {
		status = http.StatusNotFound
	}
	write(w, status, map[string]string{"error": err.Error()})
}

// logging records one line per request. The daemon runs on loopback, so the
// log is the only trace of what an agent did.
func logging(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"query", strings.TrimSpace(r.URL.RawQuery),
			"status", rec.status)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
