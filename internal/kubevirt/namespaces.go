package kubevirt

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/zyvorai/kryton/internal/kubeapi"
)

// EnsureNamespaces creates Kubernetes namespaces for each Kryton project.
func EnsureNamespaces(ctx context.Context, client *kubeapi.Client, prefix string, projects []string) error {
	for _, project := range projects {
		ns := prefix + project
		if err := ensureNamespace(ctx, client, ns, project); err != nil {
			return fmt.Errorf("project %q: %w", project, err)
		}
	}
	return nil
}

func ensureNamespace(ctx context.Context, client *kubeapi.Client, ns, project string) error {
	path := fmt.Sprintf("/api/v1/namespaces/%s", url.PathEscape(ns))
	var existing map[string]any
	if err := client.JSON(ctx, http.MethodGet, path, "", nil, &existing); err == nil {
		return nil
	} else if !kubeapi.IsNotFound(err) {
		return err
	}
	body := map[string]any{
		"apiVersion": "v1",
		"kind":       "Namespace",
		"metadata": map[string]any{
			"name": ns,
			"labels": map[string]any{
				"app.kubernetes.io/managed-by": "kryton",
				"kryton.io/project":            project,
			},
		},
	}
	return client.JSON(ctx, http.MethodPost, "/api/v1/namespaces", "application/json", body, nil)
}

// MissingNamespaces returns project namespaces that do not exist yet.
func MissingNamespaces(ctx context.Context, client *kubeapi.Client, prefix string, projects []string) ([]string, error) {
	var missing []string
	for _, project := range projects {
		ns := strings.TrimSpace(prefix + project)
		path := fmt.Sprintf("/api/v1/namespaces/%s", url.PathEscape(ns))
		var out map[string]any
		if err := client.JSON(ctx, http.MethodGet, path, "", nil, &out); err != nil {
			if kubeapi.IsNotFound(err) {
				missing = append(missing, ns)
				continue
			}
			return nil, err
		}
	}
	return missing, nil
}
