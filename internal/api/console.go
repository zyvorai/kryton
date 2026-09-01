package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/zyvorai/kryton/internal/auth"
	"github.com/zyvorai/kryton/internal/provider"
)

func (s *Server) machineConsole(w http.ResponseWriter, r *http.Request) {
	project, ok := s.requireProject(w, r, auth.Viewer)
	if !ok {
		return
	}
	machineID := r.PathValue("id")
	resolver, ok := s.p.(provider.ConsoleResolver)
	if !ok {
		s.writeAPIError(w, r, http.StatusNotImplemented, "UNSUPPORTED", "console is not available for this provider", "Use the dockur or kubevirt provider with console enabled.")
		return
	}
	wantHTML := strings.Contains(r.Header.Get("Accept"), "text/html") || r.URL.Query().Get("format") == "html"
	target, err := resolver.ConsoleTarget(r.Context(), project, machineID)
	if err != nil {
		if wantHTML {
			s.writeConsoleHTML(w, machineID, project, err.Error())
			return
		}
		s.writeErr(w, r, err)
		return
	}
	if target.Kind == "web" {
		s.proxyWebConsole(w, r, target.UpstreamURL, machineID)
		return
	}
	if wantHTML {
		s.writeConsoleHTML(w, machineID, project, "")
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{
		"kind":         target.Kind,
		"websocketUrl": fmt.Sprintf("/api/v1/machines/%s/vnc?project=%s", machineID, project),
		"namespace":    target.Namespace,
		"name":         target.Name,
	})
}

func (s *Server) writeConsoleHTML(w http.ResponseWriter, machineID, project, connectErr string) {
	cfg, _ := json.Marshal(map[string]string{
		"machine":   machineID,
		"project":   project,
		"bootError": connectErr,
	})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(w, consoleHTML, string(cfg), machineID)
}

func (s *Server) machineVNC(w http.ResponseWriter, r *http.Request) {
	project, ok := s.requireProject(w, r, auth.Viewer)
	if !ok {
		return
	}
	if s.kubeClient == nil {
		s.writeAPIError(w, r, http.StatusNotImplemented, "UNSUPPORTED", "vnc proxy requires kubevirt provider", "Open the web console URL for dockur guests, or use kubevirt for VNC.")
		return
	}
	resolver, ok := s.p.(provider.ConsoleResolver)
	if !ok {
		s.writeAPIError(w, r, http.StatusNotImplemented, "UNSUPPORTED", "console is not available for this provider", "Use the dockur or kubevirt provider with console enabled.")
		return
	}
	target, err := resolver.ConsoleTarget(r.Context(), project, r.PathValue("id"))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	if err := s.kubeClient.ProxyVNC(w, r, target.Namespace, target.Name); err != nil && s.log != nil {
		s.log.Warn("vnc proxy closed", "error", err)
	}
}

func (s *Server) proxyWebConsole(w http.ResponseWriter, r *http.Request, upstream, machineID string) {
	target, err := url.Parse(strings.TrimRight(upstream, "/"))
	if err != nil || target.Host == "" {
		s.writeAPIError(w, r, http.StatusBadGateway, "CONSOLE_UNAVAILABLE", "invalid dockur console upstream", "Confirm the dockur Windows container is running and publishing its web console port.")
		return
	}
	prefix := "/api/v1/machines/" + machineID + "/console"
	proxy := httputil.NewSingleHostReverseProxy(target)
	origDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		origDirector(req)
		suffix := strings.TrimPrefix(req.URL.Path, prefix)
		if suffix == "" {
			suffix = "/"
		}
		req.URL.Path = suffix
		req.URL.RawPath = ""
		req.Host = target.Host
	}
	proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
		if s.log != nil {
			s.log.Warn("dockur console proxy error", "machine", machineID, "error", err)
		}
		http.Error(rw, "Console proxy unavailable", http.StatusBadGateway)
	}
	proxy.ServeHTTP(w, r)
}

// consoleHTML args: title machineID, config JSON
const consoleHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Kryton Console</title>
<style>
html,body{margin:0;height:100%%;background:#111;color:#e8ecf2;font-family:"Source Sans 3",ui-sans-serif,sans-serif}
#bar{padding:10px 14px;border-bottom:1px solid #333;font-size:13px;display:flex;justify-content:space-between;gap:12px;align-items:center;flex-wrap:wrap}
#bar .actions{display:flex;gap:8px;flex-wrap:wrap;align-items:center}
#hint{margin:0;padding:8px 14px;font-size:12px;color:#a8b2c3;border-bottom:1px solid #222;background:#16161a}
#screen{height:calc(100%% - 78px);position:relative;background:#000}#viewport{position:absolute;inset:0}
#err{display:none;position:absolute;inset:0;place-items:center;padding:24px;text-align:center;background:#111;color:#a8b2c3;font-size:14px;line-height:1.5;z-index:2}
#err.show{display:grid}#err strong{display:block;color:#fff;font-size:16px;margin-bottom:8px}
button{border:0;border-radius:8px;padding:6px 12px;background:#2a2a30;color:#fff;cursor:pointer;font:inherit}
button:disabled{opacity:.45;cursor:not-allowed}
button.primary{background:#cc420a}
</style>
<script type="application/json" id="console-config">%s</script>
<script type="module" src="/console-viewer.js"></script>
</head>
<body>
<div id="bar">
  <span id="status">Kryton Windows console · %s</span>
  <div class="actions">
    <button type="button" class="primary" id="btnPaste" disabled>Paste clipboard</button>
    <button type="button" id="btnCad" disabled>Ctrl+Alt+Del</button>
    <button type="button" onclick="location.reload()">Reload</button>
  </div>
</div>
<p id="hint">Click the screen to type. Use <b>Paste clipboard</b> for host→guest text (browsers block silent paste). Ctrl+Alt+Del unlocks Windows login when the session is connected.</p>
<div id="screen"><div id="viewport"></div><div id="err"></div></div>
</body></html>`
