package auth

import (
	"net/http/httptest"
	"os"
	"testing"
)

func TestCan(t *testing.T) {
	p := Principal{Name: "svc", Role: Operator, Projects: []string{"finance"}}
	if !Can(p, "finance", Viewer) || !Can(p, "finance", Operator) {
		t.Fatal("expected operator access")
	}
	if Can(p, "finance", Admin) || Can(p, "other", Viewer) {
		t.Fatal("unexpected elevated or cross-project access")
	}
}

func TestHashTokenStable(t *testing.T) {
	if HashToken("abc") != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatal("unexpected sha256")
	}
}

func TestProxyRequiresSharedSecret(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/proxy-secret"
	if err := os.WriteFile(path, []byte("shared-secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	a, err := New(Config{Mode: "proxy", TrustProxy: true, ProxySecretFile: path})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Kryton-Proxy-Secret", "shared-secret")
	r.Header.Set("X-Kryton-User", "alice")
	r.Header.Set("X-Kryton-Role", "viewer")
	r.Header.Set("X-Kryton-Projects", "finance")
	p, err := a.authenticate(r)
	if err != nil || p.Name != "alice" {
		t.Fatalf("unexpected auth result: %+v %v", p, err)
	}
	r.Header.Set("X-Kryton-Proxy-Secret", "wrong")
	if _, err := a.authenticate(r); err == nil {
		t.Fatal("wrong proxy secret accepted")
	}
}
