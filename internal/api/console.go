package api

import (
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
		s.writeAPIError(w, r, http.StatusNotImplemented, "UNSUPPORTED", "console is not available for this provider")
		return
	}
	target, err := resolver.ConsoleTarget(r.Context(), project, machineID)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	if target.Kind == "web" {
		s.proxyWebConsole(w, r, target.UpstreamURL, machineID)
		return
	}
	if strings.Contains(r.Header.Get("Accept"), "text/html") || r.URL.Query().Get("format") == "html" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, consoleHTML, machineID, project, machineID)
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{
		"kind":         target.Kind,
		"websocketUrl": fmt.Sprintf("/api/v1/machines/%s/vnc?project=%s", machineID, project),
		"namespace":    target.Namespace,
		"name":         target.Name,
	})
}

func (s *Server) machineVNC(w http.ResponseWriter, r *http.Request) {
	project, ok := s.requireProject(w, r, auth.Viewer)
	if !ok {
		return
	}
	if s.kubeClient == nil {
		s.writeAPIError(w, r, http.StatusNotImplemented, "UNSUPPORTED", "vnc proxy requires kubevirt provider")
		return
	}
	resolver, ok := s.p.(provider.ConsoleResolver)
	if !ok {
		s.writeAPIError(w, r, http.StatusNotImplemented, "UNSUPPORTED", "console is not available for this provider")
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
		s.writeAPIError(w, r, http.StatusBadGateway, "CONSOLE_UNAVAILABLE", "invalid dockur console upstream")
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

const consoleHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Kryton Console</title>
<style>html,body{margin:0;height:100%%;background:#111;color:#f5f5f7;font-family:system-ui,sans-serif}#bar{padding:10px 14px;border-bottom:1px solid #333;font-size:13px}#screen{height:calc(100%% - 42px)}</style>
<script type="module">
import RFB from 'https://cdn.jsdelivr.net/npm/@novnc/novnc@1.5.0/core/rfb.js';
const proto=location.protocol==='https:'?'wss:':'ws:';
const url=proto+'//'+location.host+'/api/v1/machines/%s/vnc?project=%s';
const rfb=new RFB(document.getElementById('screen'),url,{shared:true});
rfb.scaleViewport=true;rfb.resizeSession=true;
</script></head><body><div id="bar">Kryton Windows console · machine %s</div><div id="screen"></div></body></html>`
