package api

import (
	"net/http"
	"os"
	"strings"
)

type labBootstrapResponse struct {
	AutoAuth bool   `json:"autoAuth"`
	Token    string `json:"token,omitempty"`
}

// labBootstrap returns the lab bearer token for browser auto-login.
// Enabled only when KRYTON_LAB_AUTO_AUTH=true, KRYTON_ALLOW_INSECURE=true, and auth mode is apikey.
func (s *Server) labBootstrap(w http.ResponseWriter, r *http.Request) {
	if !s.labAutoAuth || !s.allowInsecure || s.authMode != "apikey" || strings.TrimSpace(s.labTokenFile) == "" {
		jsonResponse(w, http.StatusOK, labBootstrapResponse{AutoAuth: false})
		return
	}
	b, err := os.ReadFile(s.labTokenFile)
	if err != nil {
		jsonResponse(w, http.StatusOK, labBootstrapResponse{AutoAuth: false})
		return
	}
	tok := strings.TrimSpace(string(b))
	if tok == "" {
		jsonResponse(w, http.StatusOK, labBootstrapResponse{AutoAuth: false})
		return
	}
	jsonResponse(w, http.StatusOK, labBootstrapResponse{AutoAuth: true, Token: tok})
}
