package kubeapi

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type Config struct {
	Endpoint           string
	BearerToken        string
	TokenFile          string
	CAFile             string
	InsecureSkipVerify bool
}

type Client struct {
	base  *url.URL
	token string
	http  *http.Client
}

type APIError struct {
	StatusCode int
	Reason     string
	Message    string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("kubernetes API returned HTTP %d", e.StatusCode)
}

func New(cfg Config) (*Client, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")
	if endpoint == "" {
		host := os.Getenv("KUBERNETES_SERVICE_HOST")
		port := os.Getenv("KUBERNETES_SERVICE_PORT_HTTPS")
		if port == "" {
			port = os.Getenv("KUBERNETES_SERVICE_PORT")
		}
		if host == "" {
			return nil, errors.New("Kubernetes endpoint is unset and in-cluster service environment is unavailable")
		}
		if port == "" {
			port = "443"
		}
		endpoint = "https://" + host + ":" + port
	}
	base, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse Kubernetes endpoint: %w", err)
	}

	token := strings.TrimSpace(cfg.BearerToken)
	if token == "" && cfg.TokenFile != "" {
		if b, err := os.ReadFile(cfg.TokenFile); err == nil {
			token = strings.TrimSpace(string(b))
		}
	}

	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: cfg.InsecureSkipVerify} //nolint:gosec -- explicit operator setting
	if cfg.CAFile != "" {
		if pem, err := os.ReadFile(cfg.CAFile); err == nil {
			pool, err := x509.SystemCertPool()
			if err != nil || pool == nil {
				pool = x509.NewCertPool()
			}
			if !pool.AppendCertsFromPEM(pem) {
				return nil, fmt.Errorf("no certificates found in %s", cfg.CAFile)
			}
			tlsConfig.RootCAs = pool
		} else if base.Scheme == "https" && !cfg.InsecureSkipVerify {
			return nil, fmt.Errorf("read Kubernetes CA file: %w", err)
		}
	}
	transport := &http.Transport{Proxy: http.ProxyFromEnvironment, TLSClientConfig: tlsConfig, MaxIdleConns: 100, IdleConnTimeout: 90 * time.Second, TLSHandshakeTimeout: 10 * time.Second}
	return &Client{base: base, token: token, http: &http.Client{Transport: transport, Timeout: 20 * time.Second}}, nil
}

func (c *Client) Health(ctx context.Context) error {
	var out map[string]any
	return c.JSON(ctx, http.MethodGet, "/version", "", nil, &out)
}

func (c *Client) JSON(ctx context.Context, method, path, contentType string, body, out any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	u := *c.base
	u.Path = strings.TrimRight(c.base.Path, "/") + pathOnly(path)
	u.RawQuery = queryOnly(path)
	req, err := http.NewRequestWithContext(ctx, method, u.String(), r)
	if err != nil {
		return err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if body != nil {
		if contentType == "" {
			contentType = "application/json"
		}
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "kryton/1.0")
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		apiErr := &APIError{StatusCode: res.StatusCode}
		var status struct{ Reason, Message string }
		if json.Unmarshal(payload, &status) == nil {
			apiErr.Reason, apiErr.Message = status.Reason, status.Message
		}
		if apiErr.Message == "" {
			apiErr.Message = strings.TrimSpace(string(payload))
		}
		return apiErr
	}
	if out != nil && len(payload) > 0 {
		if err := json.Unmarshal(payload, out); err != nil {
			return fmt.Errorf("decode Kubernetes response: %w", err)
		}
	}
	return nil
}

func IsNotFound(err error) bool {
	var e *APIError
	return errors.As(err, &e) && e.StatusCode == http.StatusNotFound
}
func IsConflict(err error) bool {
	var e *APIError
	return errors.As(err, &e) && e.StatusCode == http.StatusConflict
}

func pathOnly(v string) string {
	if i := strings.IndexByte(v, '?'); i >= 0 {
		return v[:i]
	}
	return v
}
func queryOnly(v string) string {
	if i := strings.IndexByte(v, '?'); i >= 0 {
		return v[i+1:]
	}
	return ""
}
