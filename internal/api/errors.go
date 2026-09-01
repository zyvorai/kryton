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

package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/zyvorai/kryton/internal/kubeapi"
	"github.com/zyvorai/kryton/internal/provider"
)

type classifiedError struct {
	Status  int
	Code    string
	Message string
	Hint    string
}

// classifyError maps provider/Kubernetes failures to API responses with guidance.
func classifyError(err error) classifiedError {
	if err == nil {
		return classifiedError{Status: http.StatusInternalServerError, Code: "INTERNAL", Message: "unknown error"}
	}
	switch {
	case errors.Is(err, provider.ErrNotFound):
		return classifiedError{Status: http.StatusNotFound, Code: "NOT_FOUND", Message: "resource not found", Hint: "Refresh the machine list, or check the project query parameter."}
	case errors.Is(err, provider.ErrConflict):
		return classifiedError{Status: http.StatusConflict, Code: "CONFLICT", Message: "resource already exists", Hint: "Choose a different machine name in this project."}
	case errors.Is(err, provider.ErrUnsupported):
		return classifiedError{
			Status:  http.StatusNotImplemented,
			Code:    "UNSUPPORTED",
			Message: strings.TrimPrefix(err.Error(), provider.ErrUnsupported.Error()+": "),
			Hint:    "This operation is not supported by the active provider.",
		}
	}

	msg := strings.TrimSpace(err.Error())
	var apiErr *kubeapi.APIError
	if errors.As(err, &apiErr) && apiErr != nil {
		if m := strings.TrimSpace(apiErr.Message); m != "" {
			msg = m
		} else if apiErr.Error() != "" {
			msg = apiErr.Error()
		}
		if c := classifyMessage(msg); c.Code != "" {
			if apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 && c.Status >= 500 {
				c.Status = http.StatusBadRequest
				if c.Code == "PROVIDER_UNAVAILABLE" {
					c.Code = "INVALID_REQUEST"
				}
			}
			return c
		}
		if apiErr.StatusCode == http.StatusConflict {
			return classifiedError{Status: http.StatusConflict, Code: "CONFLICT", Message: msg, Hint: "Choose a different machine name or delete the existing VM first."}
		}
		if apiErr.StatusCode == http.StatusNotFound {
			return classifiedError{Status: http.StatusBadRequest, Code: "INVALID_REQUEST", Message: msg, Hint: "Confirm the image DataSource, namespace, and storage class exist."}
		}
		if apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			return classifiedError{Status: http.StatusBadRequest, Code: "INVALID_REQUEST", Message: msg, Hint: "Fix the request or cluster prerequisites, then retry."}
		}
		if apiErr.StatusCode >= 500 {
			c := classifyMessage(msg)
			if c.Code != "" {
				return c
			}
			return classifiedError{
				Status:  http.StatusBadGateway,
				Code:    "PROVIDER_ERROR",
				Message: msg,
				Hint:    "Inspect Kubernetes/KubeVirt: kubectl -n kubevirt get pods; kubectl get events -A --sort-by=.lastTimestamp | tail",
			}
		}
	}

	if c := classifyMessage(msg); c.Code != "" {
		return c
	}
	if msg == "" {
		msg = "internal server error"
	}
	return classifiedError{
		Status:  http.StatusInternalServerError,
		Code:    "INTERNAL",
		Message: msg,
		Hint:    "Check krytond logs (journalctl -u kryton-kubevirt) and kubectl get events -A.",
	}
}

func classifyMessage(msg string) classifiedError {
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "datasource") && strings.Contains(lower, "not found"):
		return classifiedError{
			Status:  http.StatusBadRequest,
			Code:    "IMAGE_NOT_READY",
			Message: msg,
			Hint:    "Publish a CDI DataSource for this catalog ID: Scripts → build-golden-image.sh then bootstrap-kubevirt-images.sh (docs/GOLDEN-IMAGES.md). Until then, use the dockur provider or only Stored images in the UI.",
		}
	case strings.Contains(lower, "insufficient permissions") && (strings.Contains(lower, "clone") || strings.Contains(lower, "kryton-images")):
		return classifiedError{
			Status:  http.StatusBadRequest,
			Code:    "CLONE_FORBIDDEN",
			Message: msg,
			Hint:    "Allow project namespaces to clone from the image namespace: kubectl apply -f deploy/kubevirt/clone-rbac.yaml (or recreate the VM so Kryton auto-creates the RoleBinding).",
		}
	case strings.Contains(lower, "datavolumes/source") || (strings.Contains(lower, "authorization failed") && strings.Contains(lower, "clone")):
		return classifiedError{
			Status:  http.StatusBadRequest,
			Code:    "CLONE_FORBIDDEN",
			Message: msg,
			Hint:    "Apply deploy/kubevirt/clone-rbac.yaml so default ServiceAccounts can clone DataSources from kryton-images.",
		}
	case strings.Contains(lower, "no endpoints available") || strings.Contains(lower, "failed calling webhook") || strings.Contains(lower, "virt-api"):
		return classifiedError{
			Status:  http.StatusServiceUnavailable,
			Code:    "PROVIDER_UNAVAILABLE",
			Message: msg,
			Hint:    "KubeVirt control plane is down. Check disk pressure (df -h), then: kubectl -n kubevirt get pods; ensure virt-api has endpoints before creating VMs.",
		}
	case strings.Contains(lower, "disk pressure") || strings.Contains(lower, "evicted"):
		return classifiedError{
			Status:  http.StatusServiceUnavailable,
			Code:    "NODE_DISK_PRESSURE",
			Message: msg,
			Hint:    "Free node disk until kubelet clears DiskPressure (aim for >10% free on /), then restart pending kubevirt pods.",
		}
	case strings.Contains(lower, "admission webhook") || strings.Contains(lower, "denied the request"):
		return classifiedError{
			Status:  http.StatusBadRequest,
			Code:    "ADMISSION_DENIED",
			Message: msg,
			Hint:    "The cluster rejected the VirtualMachine. Fix the referenced image/storage/RBAC, then retry create.",
		}
	case strings.Contains(lower, "storageclass") && (strings.Contains(lower, "not found") || strings.Contains(lower, "invalid")):
		return classifiedError{
			Status:  http.StatusBadRequest,
			Code:    "STORAGE_INVALID",
			Message: msg,
			Hint:    "Pick a valid StorageClass in Settings → Cluster storage (e.g. longhorn or rook-ceph-block).",
		}
	case strings.Contains(lower, "ensure cdi clone access"):
		return classifiedError{
			Status:  http.StatusBadGateway,
			Code:    "CLONE_RBAC_FAILED",
			Message: msg,
			Hint:    "Kryton could not create clone RBAC. Apply deploy/kubevirt/clone-rbac.yaml with cluster-admin credentials.",
		}
	default:
		return classifiedError{}
	}
}
