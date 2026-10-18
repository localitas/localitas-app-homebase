package homebase

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	client "github.com/localitas/localitas-go"
	"github.com/localitas/localitas-go/identitytest"
)

const testAPIToken = "lt_test_cluster_token"

// signer signs identity tokens as core does; the app verifies them with its
// public key and refuses legacy (api_token-derived HMAC) tokens.
var signer *identitytest.Signer

func TestMain(m *testing.M) {
	os.Setenv("LOCALITAS_API_TOKEN", testAPIToken)
	var err error
	if signer, err = identitytest.Install(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// identity signs claims as core does for a request it forwards to the app.
func identity(userID, permission string) string {
	return signer.Token(client.TokenInfo{UserID: userID, Email: userID + "@test.local", Permission: permission})
}

func guarded(mux *http.ServeMux, method, path, token string) int {
	req := httptest.NewRequest(method, path, strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec.Code
}

// Device control needs a verified user, write scope to change devices and
// admin scope for plugin credentials. Before, RequireScopeFunc ran without
// AuthMiddleware and refused even admins.
func TestRoutes_RequireIdentityAndScope(t *testing.T) {
	mux := http.NewServeMux()
	newTestApp("http://localhost:1").RegisterRoutes(mux)
	forged := base64.StdEncoding.EncodeToString([]byte(`{"user_id":"x","permission":"admin"}`))

	for _, token := range []string{"", forged} {
		if got := guarded(mux, "GET", "/api/clusters", token); got != http.StatusUnauthorized {
			t.Errorf("GET /api/clusters with %q: %d, want 401", token, got)
		}
	}
	if got := guarded(mux, "GET", "/api/clusters", identity("kid", "read")); got != http.StatusOK {
		t.Errorf("a read user lists clusters: %d, want 200", got)
	}
	if got := guarded(mux, "POST", "/api/devices/d1/command", identity("kid", "read")); got != http.StatusForbidden {
		t.Errorf("a read user commands a device: %d, want 403", got)
	}
	if got := guarded(mux, "PUT", "/api/plugins/ring/credential", identity("parent", "write")); got != http.StatusForbidden {
		t.Errorf("a write user sets a plugin credential: %d, want 403", got)
	}
}

func TestPluginClient_SendsSystemToken(t *testing.T) {
	var got string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	if err := NewPluginClient(server.URL, "homebase-ring").Configure("cred-1"); err != nil {
		t.Fatal(err)
	}
	if got != "Bearer "+testAPIToken {
		t.Fatalf("plugin call carried %q, want core's token", got)
	}
}
