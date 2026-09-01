package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/zyvorai/kryton/internal/auth"
	"github.com/zyvorai/kryton/internal/catalog"
	"github.com/zyvorai/kryton/internal/doctor"
	"github.com/zyvorai/kryton/internal/events"
	"github.com/zyvorai/kryton/internal/metrics"
	"github.com/zyvorai/kryton/internal/model"
	"github.com/zyvorai/kryton/internal/provider"
)

type Config struct {
	Provider       provider.Provider
	Catalog        *catalog.Catalog
	Events         *events.Bus
	Auth           *auth.Authenticator
	Metrics        *metrics.Metrics
	Web            fs.FS
	Projects       []string
	DefaultProject string
	AuthMode       string
	DockurDataDir  string
	DockurRuntime  string
	Log            *slog.Logger
}

type Server struct {
	p              provider.Provider
	catalog        *catalog.Catalog
	events         *events.Bus
	auth           *auth.Authenticator
	metrics        *metrics.Metrics
	web            fs.FS
	projects       []string
	defaultProject string
	authMode       string
	dockurDataDir  string
	dockurRuntime  string
	log            *slog.Logger
}

type createRequest struct {
	Project string `json:"project"`
	model.MachineSpec
}
type snapshotRequest struct {
	Name string `json:"name,omitempty"`
}
type listResponse[T any] struct {
	Items []T `json:"items"`
}
type errorEnvelope struct {
	Error APIError `json:"error"`
}
type APIError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"requestId,omitempty"`
}

func New(cfg Config) *Server {
	return &Server{
		p: cfg.Provider, catalog: cfg.Catalog, events: cfg.Events, auth: cfg.Auth, metrics: cfg.Metrics, web: cfg.Web,
		projects: cfg.Projects, defaultProject: cfg.DefaultProject, authMode: cfg.AuthMode,
		dockurDataDir: cfg.DockurDataDir, dockurRuntime: cfg.DockurRuntime, log: cfg.Log,
	}
}

func (s *Server) Handler() http.Handler {
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("GET /api/v1/me", s.me)
	apiMux.HandleFunc("GET /api/v1/projects", s.listProjects)
	apiMux.HandleFunc("GET /api/v1/capabilities", s.capabilities)
	apiMux.HandleFunc("GET /api/v1/doctor", s.doctor)
	apiMux.HandleFunc("GET /api/v1/images", s.images)
	apiMux.HandleFunc("GET /api/v1/summary", s.summary)
	apiMux.HandleFunc("GET /api/v1/machines", s.listMachines)
	apiMux.HandleFunc("POST /api/v1/machines", s.createMachine)
	apiMux.HandleFunc("GET /api/v1/machines/{id}", s.getMachine)
	apiMux.HandleFunc("DELETE /api/v1/machines/{id}", s.deleteMachine)
	apiMux.HandleFunc("POST /api/v1/machines/{id}/start", s.startMachine)
	apiMux.HandleFunc("POST /api/v1/machines/{id}/stop", s.stopMachine)
	apiMux.HandleFunc("POST /api/v1/machines/{id}/snapshot", s.snapshotMachine)
	apiMux.HandleFunc("GET /api/v1/events", s.listEvents)
	apiMux.HandleFunc("GET /api/v1/events/stream", s.streamEvents)

	root := http.NewServeMux()
	root.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	root.HandleFunc("GET /readyz", s.ready)
	root.HandleFunc("GET /metrics", s.metrics.Handler)
	root.Handle("/api/", s.auth.Middleware(apiMux))
	root.Handle("/", s.staticHandler())
	return s.requestID(s.accessLog(s.security(s.recoverer(root))))
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, http.StatusOK, auth.FromContext(r.Context()))
}
func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, http.StatusOK, listResponse[string]{Items: auth.FilterProjects(auth.FromContext(r.Context()), s.projects)})
}
func (s *Server) capabilities(w http.ResponseWriter, r *http.Request) {
	c, err := s.p.Capabilities(r.Context())
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	jsonResponse(w, http.StatusOK, c)
}
func (s *Server) doctor(w http.ResponseWriter, r *http.Request) {
	report := doctor.Run(r.Context(), doctor.Input{
		Provider:  s.p,
		Catalog:   s.catalog,
		AuthMode:  s.authMode,
		Projects:  s.projects,
		DockurDir: s.dockurDataDir,
		Runtime:   s.dockurRuntime,
	})
	status := http.StatusOK
	if !report.Healthy {
		status = http.StatusServiceUnavailable
	}
	jsonResponse(w, status, report)
}
func (s *Server) images(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, http.StatusOK, listResponse[model.Image]{Items: s.catalog.List()})
}

func (s *Server) summary(w http.ResponseWriter, r *http.Request) {
	project, ok := s.requireProject(w, r, auth.Viewer)
	if !ok {
		return
	}
	ms, err := s.p.List(r.Context(), project)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	x := model.Summary{Project: project, Provider: s.p.Name(), Machines: len(ms)}
	for _, m := range ms {
		x.CPU += m.Spec.Compute.CPU
		x.MemoryMiB += m.Spec.Compute.MemoryMiB
		switch m.State {
		case model.StateRunning:
			x.Running++
		case model.StateStopped:
			x.Stopped++
		case model.StateFailed, model.StateUnknown:
			x.Attention++
		}
	}
	jsonResponse(w, http.StatusOK, x)
}

func (s *Server) listMachines(w http.ResponseWriter, r *http.Request) {
	project, ok := s.requireProject(w, r, auth.Viewer)
	if !ok {
		return
	}
	ms, err := s.p.List(r.Context(), project)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	jsonResponse(w, http.StatusOK, listResponse[model.Machine]{Items: ms})
}
func (s *Server) getMachine(w http.ResponseWriter, r *http.Request) {
	project, ok := s.requireProject(w, r, auth.Viewer)
	if !ok {
		return
	}
	m, err := s.p.Get(r.Context(), project, r.PathValue("id"))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	jsonResponse(w, http.StatusOK, m)
}

func (s *Server) createMachine(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := decodeJSON(w, r, &req); err != nil {
		s.badRequest(w, r, err.Error())
		return
	}
	if req.Project == "" {
		req.Project = s.defaultProject
	}
	if !s.projectConfigured(req.Project) {
		s.badRequest(w, r, "project is not configured")
		return
	}
	if !auth.Can(auth.FromContext(r.Context()), req.Project, auth.Operator) {
		s.forbidden(w, r)
		return
	}
	if err := model.ValidateProject(req.Project); err != nil {
		s.badRequest(w, r, err.Error())
		return
	}
	if err := model.ValidateMachineSpec(req.MachineSpec); err != nil {
		s.badRequest(w, r, err.Error())
		return
	}
	img, exists := s.catalog.Get(req.Image)
	if !exists {
		s.badRequest(w, r, "image is not present in the Kryton image catalog")
		return
	}
	if req.Compute.CPU < img.MinCPU || req.Compute.MemoryMiB < img.MinMemoryMiB {
		s.badRequest(w, r, fmt.Sprintf("image requires at least %d CPU and %d MiB memory", img.MinCPU, img.MinMemoryMiB))
		return
	}
	m, err := s.p.Create(r.Context(), req.Project, req.MachineSpec)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	s.metrics.Operation("create")
	s.events.Publish(r.Context(), "io.kryton.machine.created", "machines/"+m.ID, map[string]any{"project": m.Project, "machineId": m.ID, "name": m.Spec.Name, "state": m.State, "consoleUrl": m.ConsoleURL})
	if m.State == model.StateProvisioning {
		s.events.Publish(r.Context(), "io.kryton.machine.install.started", "machines/"+m.ID, map[string]any{"project": m.Project, "machineId": m.ID, "name": m.Spec.Name, "consoleUrl": m.ConsoleURL, "message": m.Message})
	}
	jsonResponse(w, http.StatusCreated, m)
}

func (s *Server) startMachine(w http.ResponseWriter, r *http.Request) { s.machineAction(w, r, "start") }
func (s *Server) stopMachine(w http.ResponseWriter, r *http.Request)  { s.machineAction(w, r, "stop") }
func (s *Server) machineAction(w http.ResponseWriter, r *http.Request, action string) {
	project, ok := s.requireProject(w, r, auth.Operator)
	if !ok {
		return
	}
	machineID := r.PathValue("id")
	var m *model.Machine
	var err error
	if action == "start" {
		m, err = s.p.Start(r.Context(), project, machineID)
	} else {
		m, err = s.p.Stop(r.Context(), project, machineID)
	}
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	s.metrics.Operation(action)
	eventType := "io.kryton.machine.started"
	if action == "stop" {
		eventType = "io.kryton.machine.stopped"
	}
	s.events.Publish(r.Context(), eventType, "machines/"+m.ID, map[string]any{"project": project, "machineId": m.ID, "name": m.Spec.Name, "state": m.State})
	jsonResponse(w, http.StatusOK, m)
}

func (s *Server) deleteMachine(w http.ResponseWriter, r *http.Request) {
	project, ok := s.requireProject(w, r, auth.Operator)
	if !ok {
		return
	}
	machineID := r.PathValue("id")
	m, err := s.p.Get(r.Context(), project, machineID)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	if err := s.p.Delete(r.Context(), project, machineID); err != nil {
		s.writeErr(w, r, err)
		return
	}
	s.metrics.Operation("delete")
	s.events.Publish(r.Context(), "io.kryton.machine.deleted", "machines/"+machineID, map[string]any{"project": project, "machineId": machineID, "name": m.Spec.Name})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) snapshotMachine(w http.ResponseWriter, r *http.Request) {
	project, ok := s.requireProject(w, r, auth.Operator)
	if !ok {
		return
	}
	var req snapshotRequest
	if r.ContentLength > 0 {
		if err := decodeJSON(w, r, &req); err != nil {
			s.badRequest(w, r, err.Error())
			return
		}
	}
	snap, err := s.p.Snapshot(r.Context(), project, r.PathValue("id"), req.Name)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	s.metrics.Operation("snapshot")
	s.events.Publish(r.Context(), "io.kryton.snapshot.created", "snapshots/"+snap.ID, map[string]any{"project": project, "machineId": snap.MachineID, "snapshotId": snap.ID, "name": snap.Name})
	jsonResponse(w, http.StatusCreated, snap)
}

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	p := auth.FromContext(r.Context())
	limit := 50
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 500 {
		limit = v
	}
	all := s.events.List(500)
	visible := make([]events.Event, 0, limit)
	for _, e := range all {
		if s.eventVisible(p, e) {
			visible = append(visible, e)
			if len(visible) == limit {
				break
			}
		}
	}
	jsonResponse(w, http.StatusOK, listResponse[events.Event]{Items: visible})
}
func (s *Server) streamEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeAPIError(w, r, http.StatusNotImplemented, "STREAM_UNSUPPORTED", "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	p := auth.FromContext(r.Context())
	ch, cancel := s.events.Subscribe()
	defer cancel()
	fmt.Fprint(w, ": kryton event stream\n\n")
	flusher.Flush()
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		case e, open := <-ch:
			if !open {
				return
			}
			if !s.eventVisible(p, e) {
				continue
			}
			b, _ := json.Marshal(e)
			fmt.Fprintf(w, "id: %s\nevent: cloudevent\ndata: %s\n\n", e.ID, b)
			flusher.Flush()
		}
	}
}
func (s *Server) eventVisible(p auth.Principal, e events.Event) bool {
	project, _ := e.Data["project"].(string)
	return project == "" || auth.Can(p, project, auth.Viewer)
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.p.Health(ctx); err != nil {
		jsonResponse(w, http.StatusServiceUnavailable, map[string]any{"status": "not-ready", "provider": s.p.Name()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"status": "ready", "provider": s.p.Name()})
}

func (s *Server) requireProject(w http.ResponseWriter, r *http.Request, role auth.Role) (string, bool) {
	project := r.URL.Query().Get("project")
	if project == "" {
		project = s.defaultProject
	}
	if !s.projectConfigured(project) {
		s.badRequest(w, r, "project is not configured")
		return "", false
	}
	if !auth.Can(auth.FromContext(r.Context()), project, role) {
		s.forbidden(w, r)
		return "", false
	}
	return project, true
}
func (s *Server) projectConfigured(project string) bool {
	for _, p := range s.projects {
		if p == project {
			return true
		}
	}
	return false
}

func (s *Server) writeErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, provider.ErrNotFound):
		s.writeAPIError(w, r, http.StatusNotFound, "NOT_FOUND", "resource not found")
	case errors.Is(err, provider.ErrConflict):
		s.writeAPIError(w, r, http.StatusConflict, "CONFLICT", "resource already exists")
	case errors.Is(err, provider.ErrUnsupported):
		s.writeAPIError(w, r, http.StatusNotImplemented, "UNSUPPORTED", "operation unsupported")
	default:
		s.log.Error("request failed", "request_id", requestID(r.Context()), "error", err)
		s.writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL", "internal server error")
	}
}
func (s *Server) badRequest(w http.ResponseWriter, r *http.Request, msg string) {
	s.writeAPIError(w, r, http.StatusBadRequest, "INVALID_REQUEST", msg)
}
func (s *Server) forbidden(w http.ResponseWriter, r *http.Request) {
	s.writeAPIError(w, r, http.StatusForbidden, "FORBIDDEN", "you do not have access to this project or operation")
}
func (s *Server) writeAPIError(w http.ResponseWriter, r *http.Request, status int, code, msg string) {
	jsonResponse(w, status, errorEnvelope{Error: APIError{Code: code, Message: msg, RequestID: requestID(r.Context())}})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return errors.New("request body must contain one JSON object")
	}
	return nil
}
func jsonResponse(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if status != http.StatusNoContent {
		_ = json.NewEncoder(w).Encode(v)
	}
}
