package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func runCLI(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(context.Background(), args, strings.NewReader(stdin), &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestVerifyWebhookVector(t *testing.T) {
	body := filepath.Join(t.TempDir(), "body.json")
	if err := os.WriteFile(body, []byte(`{"id":"evt_test","type":"tracking.updated","data":{"id":"trk_123"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	hdr := "t=1792560000,v1=19752dd2fca1872cc8aa6cc36c412fce3bbfa120480580bdb1ad9d7957dfc9f4"
	// The vector's timestamp is fixed, so widen the tolerance instead of pinning the clock.
	code, out, _ := runCLI(t, "", "verify-webhook", "--secret", "whsec_trackified_test_vector", "--header", hdr, "--body-file", body, "--tolerance", "876000h", "--json")
	if code != 0 || !strings.Contains(out, `"valid": true`) {
		t.Fatalf("code=%d out=%s", code, out)
	}
	code, _, _ = runCLI(t, "", "verify-webhook", "--secret", "wrong", "--header", hdr, "--body-file", body, "--tolerance", "876000h")
	if code != 1 {
		t.Fatalf("wrong secret: code=%d", code)
	}
	code, _, _ = runCLI(t, "", "verify-webhook", "--secret", "whsec_trackified_test_vector", "--header", hdr, "--body-file", body)
	if code != 1 {
		t.Fatalf("stale timestamp: code=%d", code)
	}
}

func TestLoginStoresKeyPrivately(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tk_test_abc" {
			w.WriteHeader(401)
			return
		}
		_, _ = w.Write([]byte(`{"plan":"free","period_start":"2026-11-01T00:00:00Z","period_end":"2026-12-01T00:00:00Z"}`))
	}))
	defer srv.Close()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("AppData", dir)
	t.Setenv("TRACKIFIED_API_KEY", "")
	t.Setenv("TRACKIFIED_BASE_URL", srv.URL)

	if code, _, errOut := runCLI(t, "tk_test_abc\n", "login"); code != 0 {
		t.Fatalf("login failed: %s", errOut)
	}
	p, _ := configPath()
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", fi.Mode().Perm())
	}
	if got := loadAPIKey(); got != "tk_test_abc" {
		t.Fatalf("loadAPIKey = %q", got)
	}
	if code, _, _ := runCLI(t, "tk_test_bad\n", "login"); code != 1 {
		t.Fatal("bad key should fail")
	}
}

func TestListJSONAndFlagsAfterArgs(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/trackings":
			gotQuery = r.URL.RawQuery
			_, _ = w.Write([]byte(`{"data":[{"id":"trk_1","tracking_number":"123","status":"delivered","created_at":"2026-11-24T00:00:00Z","updated_at":"2026-11-24T00:00:00Z"}],"pagination":{"limit":50,"offset":0,"total":1}}`))
		case "/v1/trackings/trk_1":
			_, _ = w.Write([]byte(`{"id":"trk_1","tracking_number":"123","status":"delivered","events":[{"occurred_at":"2026-11-23T10:00:00Z","status":"delivered","description":"Delivered","location":"NYC"}],"created_at":"2026-11-24T00:00:00Z","updated_at":"2026-11-24T00:00:00Z"}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	t.Setenv("TRACKIFIED_API_KEY", "tk_test_x")
	t.Setenv("TRACKIFIED_BASE_URL", srv.URL)

	code, out, errOut := runCLI(t, "", "list", "--status", "delivered", "--all", "--json")
	var items []map[string]any
	if code != 0 || json.Unmarshal([]byte(out), &items) != nil || len(items) != 1 || !strings.Contains(gotQuery, "status=delivered") {
		t.Fatalf("code=%d out=%s err=%s q=%s", code, out, errOut, gotQuery)
	}
	// watch stops once delivered; flag after the positional argument.
	code, out, _ = runCLI(t, "", "watch", "trk_1", "--interval", "5s")
	if code != 0 || !strings.Contains(out, "Delivered") {
		t.Fatalf("watch code=%d out=%s", code, out)
	}
	if code, _, _ := runCLI(t, "", "get"); code != 2 {
		t.Fatalf("missing arg should be a usage error, got %d", code)
	}
}
