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
	"testing"

	"github.com/zyvorai/kryton/internal/kubeapi"
	"github.com/zyvorai/kryton/internal/provider"
)

func TestClassifyErrorActionable(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		wantHint   string
	}{
		{
			name:       "datasource missing",
			err:        &kubeapi.APIError{StatusCode: 422, Message: `admission webhook "virtualmachine-validator.kubevirt.io" denied the request: datasource.cdi.kubevirt.io "kryton-images/windows-11-enterprise" not found`},
			wantStatus: http.StatusBadRequest,
			wantCode:   "IMAGE_NOT_READY",
			wantHint:   "DataSource",
		},
		{
			name:       "clone rbac",
			err:        &kubeapi.APIError{StatusCode: 422, Message: `admission webhook "virtualmachine-validator.kubevirt.io" denied the request: Authorization failed, message is: User system:serviceaccount:default:default has insufficient permissions in clone source namespace kryton-images`},
			wantStatus: http.StatusBadRequest,
			wantCode:   "CLONE_FORBIDDEN",
			wantHint:   "clone-rbac",
		},
		{
			name:       "virt-api down",
			err:        &kubeapi.APIError{StatusCode: 500, Message: `Internal error occurred: failed calling webhook "virtualmachines-mutator.kubevirt.io": failed to call webhook: Post "https://virt-api.kubevirt.svc:443/virtualmachines-mutate?timeout=10s": no endpoints available for service "virt-api"`},
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   "PROVIDER_UNAVAILABLE",
			wantHint:   "virt-api",
		},
		{
			name:       "not found provider",
			err:        provider.ErrNotFound,
			wantStatus: http.StatusNotFound,
			wantCode:   "NOT_FOUND",
		},
		{
			name:       "generic still exposes message",
			err:        errors.New("something exploded in provider"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "INTERNAL",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyError(tc.err)
			if got.Status != tc.wantStatus || got.Code != tc.wantCode {
				t.Fatalf("status/code got %d/%s want %d/%s message=%q", got.Status, got.Code, tc.wantStatus, tc.wantCode, got.Message)
			}
			if got.Message == "" || got.Message == "internal server error" && tc.wantCode != "INTERNAL" {
				t.Fatalf("expected concrete message, got %q", got.Message)
			}
			if tc.wantCode == "INTERNAL" && !strings.Contains(got.Message, "something exploded") {
				t.Fatalf("generic errors must surface err text, got %q", got.Message)
			}
			if tc.wantHint != "" && !strings.Contains(got.Hint, tc.wantHint) {
				t.Fatalf("hint %q missing %q", got.Hint, tc.wantHint)
			}
			if got.Hint == "" {
				t.Fatal("expected non-empty hint")
			}
		})
	}
}
