package kubeapi

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"
)

var vncUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// ProxyVNC upgrades the client connection and relays KubeVirt VNC from the API server.
func (c *Client) ProxyVNC(w http.ResponseWriter, r *http.Request, namespace, vmi string) error {
	clientConn, err := vncUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return err
	}
	defer clientConn.Close()

	path := fmt.Sprintf("/apis/subresources.kubevirt.io/v1/namespaces/%s/virtualmachineinstances/%s/vnc", url.PathEscape(namespace), url.PathEscape(vmi))
	u := *c.base
	u.Scheme = strings.Replace(u.Scheme, "http", "ws", 1)
	u.Path = strings.TrimRight(c.base.Path, "/") + path

	header := http.Header{}
	if c.token != "" {
		header.Set("Authorization", "Bearer "+c.token)
	}
	dialer := websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: c.http.Timeout,
	}
	if tr, ok := c.http.Transport.(*http.Transport); ok && tr.TLSClientConfig != nil {
		dialer.TLSClientConfig = tr.TLSClientConfig.Clone()
	} else if u.Scheme == "wss" {
		dialer.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12} //nolint:gosec
	}
	backend, resp, err := dialer.Dial(u.String(), header)
	if err != nil {
		if resp != nil {
			_ = resp.Body.Close()
		}
		return fmt.Errorf("dial kubevirt vnc: %w", err)
	}
	defer backend.Close()

	errCh := make(chan error, 2)
	copyWS := func(dst, src *websocket.Conn) {
		for {
			mt, msg, err := src.ReadMessage()
			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) && !isNetClosed(err) {
					errCh <- err
				}
				return
			}
			if err := dst.WriteMessage(mt, msg); err != nil {
				errCh <- err
				return
			}
		}
	}
	go copyWS(backend, clientConn)
	go copyWS(clientConn, backend)
	err = <-errCh
	return err
}

func isNetClosed(err error) bool {
	if err == nil {
		return false
	}
	if err == io.EOF {
		return true
	}
	if ne, ok := err.(net.Error); ok && !ne.Timeout() {
		return strings.Contains(strings.ToLower(err.Error()), "use of closed network connection")
	}
	return false
}
