// Copyright 2026 Kryton contributors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package kubevirt

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/zyvorai/kryton/internal/kubeapi"
)

const imageCloneRole = "kryton-datavolume-cloner"

// EnsureNamespaces creates Kubernetes namespaces for each Kryton project and
// grants CDI DataSource clone access from the image namespace.
func EnsureNamespaces(ctx context.Context, client *kubeapi.Client, prefix string, projects []string) error {
	for _, project := range projects {
		ns := prefix + project
		if err := ensureNamespace(ctx, client, ns, project); err != nil {
			return fmt.Errorf("project %q: %w", project, err)
		}
	}
	return nil
}

// EnsureImageCloneAccess lets each project namespace's default SA clone DataVolumes
// from imageNamespace (required by CDI when VMs reference a DataSource).
func EnsureImageCloneAccess(ctx context.Context, client *kubeapi.Client, imageNamespace, prefix string, projects []string) error {
	imageNamespace = strings.TrimSpace(imageNamespace)
	if imageNamespace == "" {
		imageNamespace = "kryton-images"
	}
	if err := ensureNamespace(ctx, client, imageNamespace, "images"); err != nil {
		return err
	}
	if err := ensureCloneRole(ctx, client, imageNamespace); err != nil {
		return err
	}
	for _, project := range projects {
		targetNS := strings.TrimSpace(prefix + project)
		if targetNS == "" {
			continue
		}
		if err := ensureCloneRoleBinding(ctx, client, imageNamespace, targetNS); err != nil {
			return fmt.Errorf("clone rbac for %s: %w", targetNS, err)
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

func ensureCloneRole(ctx context.Context, client *kubeapi.Client, imageNS string) error {
	path := fmt.Sprintf("/apis/rbac.authorization.k8s.io/v1/namespaces/%s/roles/%s", url.PathEscape(imageNS), url.PathEscape(imageCloneRole))
	var existing map[string]any
	if err := client.JSON(ctx, http.MethodGet, path, "", nil, &existing); err == nil {
		return nil
	} else if !kubeapi.IsNotFound(err) {
		return err
	}
	body := map[string]any{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "Role",
		"metadata": map[string]any{
			"name":      imageCloneRole,
			"namespace": imageNS,
			"labels":    map[string]any{"app.kubernetes.io/managed-by": "kryton"},
		},
		"rules": []any{
			map[string]any{
				"apiGroups": []any{"cdi.kubevirt.io"},
				"resources": []any{"datavolumes/source"},
				"verbs":     []any{"create"},
			},
		},
	}
	createPath := fmt.Sprintf("/apis/rbac.authorization.k8s.io/v1/namespaces/%s/roles", url.PathEscape(imageNS))
	return client.JSON(ctx, http.MethodPost, createPath, "application/json", body, nil)
}

func ensureCloneRoleBinding(ctx context.Context, client *kubeapi.Client, imageNS, targetNS string) error {
	name := "kryton-allow-clone-from-" + sanitizeDNS(targetNS, 40)
	path := fmt.Sprintf("/apis/rbac.authorization.k8s.io/v1/namespaces/%s/rolebindings/%s", url.PathEscape(imageNS), url.PathEscape(name))
	var existing map[string]any
	if err := client.JSON(ctx, http.MethodGet, path, "", nil, &existing); err == nil {
		return nil
	} else if !kubeapi.IsNotFound(err) {
		return err
	}
	body := map[string]any{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "RoleBinding",
		"metadata": map[string]any{
			"name":      name,
			"namespace": imageNS,
			"labels":    map[string]any{"app.kubernetes.io/managed-by": "kryton"},
		},
		"subjects": []any{
			map[string]any{"kind": "ServiceAccount", "name": "default", "namespace": targetNS},
		},
		"roleRef": map[string]any{
			"apiGroup": "rbac.authorization.k8s.io",
			"kind":     "Role",
			"name":     imageCloneRole,
		},
	}
	createPath := fmt.Sprintf("/apis/rbac.authorization.k8s.io/v1/namespaces/%s/rolebindings", url.PathEscape(imageNS))
	return client.JSON(ctx, http.MethodPost, createPath, "application/json", body, nil)
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
