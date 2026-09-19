package httpapi_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zeyad/whatsapp-gateway/internal/auth"
	"github.com/zeyad/whatsapp-gateway/internal/config"
	"github.com/zeyad/whatsapp-gateway/internal/crypto"
	"github.com/zeyad/whatsapp-gateway/internal/database"
	"github.com/zeyad/whatsapp-gateway/internal/httpapi"
	"github.com/zeyad/whatsapp-gateway/internal/instance"
	"github.com/zeyad/whatsapp-gateway/internal/message"
	"github.com/zeyad/whatsapp-gateway/internal/testutil"
	"github.com/zeyad/whatsapp-gateway/internal/webhook"
	"github.com/zeyad/whatsapp-gateway/internal/whatsapp"
)

const testAdminKey = "test-admin-key-that-is-long-enough-1234567890"

type testEnv struct {
	server     *httptest.Server
	env        *testutil.TestDB
	repo       *database.InstanceRepo
	keys       *database.APIKeyRepo
	manager    *whatsapp.Manager
	dispatcher *webhook.Dispatcher
}

func newTestEnv(t *testing.T, name string, adminKey string, rateLimit float64, rateBurst int) *testEnv {
	t.Helper()
	tdb := testutil.RequireDB(t, name)
	repo := database.NewInstanceRepo(tdb.DB)
	keys := database.NewAPIKeyRepo(tdb.DB)
	box, err := crypto.New(strings.Repeat("k", 48))
	if err != nil {
		t.Fatalf("crypto.New: %v", err)
	}
	dispatcher := webhook.NewDispatcher(webhook.Config{
		Timeout:    time.Second,
		MaxRetries: 0,
		Workers:    1,
		QueueSize:  8,
		Log:        tdb.Log,
	}, repo, box)
	manager := whatsapp.NewManager(whatsapp.ManagerOptions{
		Container:    tdb.Container,
		Repo:         repo,
		Notifier:     dispatcher,
		Log:          tdb.Log,
		SendTimeout:  2 * time.Second,
		MaxInstances: 50,
	})
	instSvc := instance.NewService(repo, keys, manager, box, tdb.Log)
	msgSvc := message.NewService(manager, 1<<20, tdb.Log)
	cfg := &config.Config{
		AppEnv:              "development",
		AdminAPIKey:         adminKey,
		RateLimitPerSecond:  rateLimit,
		RateLimitBurst:      rateBurst,
		AdminRateLimitRPS:   100,
		AdminRateLimitBurst: 100,
		MaxUploadBytes:      1 << 20,
	}
	srv := httpapi.New(httpapi.Deps{
		Config:    cfg,
		Log:       tdb.Log,
		DB:        tdb.DB,
		Keys:      keys,
		Instances: instSvc,
		Messages:  msgSvc,
		Manager:   manager,
	})
	ts := httptest.NewServer(srv.Engine())
	t.Cleanup(func() {
		ts.Close()
		dispatcher.Shutdown(time.Second)
		manager.Shutdown(time.Second)
	})
	return &testEnv{server: ts, env: tdb, repo: repo, keys: keys, manager: manager, dispatcher: dispatcher}
}

func (e *testEnv) do(t *testing.T, method, path, key string, body any) (*http.Response, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, e.server.URL+path, reader)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := e.server.Client().Do(req)
	if err != nil {
		t.Fatalf("request %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp, raw
}

func decode[T any](t *testing.T, raw []byte) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode %s: %v", string(raw), err)
	}
	return out
}

func createInstance(t *testing.T, e *testEnv, shopID, webhookURL string) map[string]any {
	t.Helper()
	body := map[string]any{"shop_id": shopID, "display_name": "محل " + shopID}
	if webhookURL != "" {
		body["webhook_url"] = webhookURL
	}
	resp, raw := e.do(t, http.MethodPost, "/api/v1/admin/instances", testAdminKey, body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create instance status = %d body=%s", resp.StatusCode, raw)
	}
	return decode[map[string]any](t, raw)
}

func TestHealthEndpoint(t *testing.T) {
	e := newTestEnv(t, "httpapi_health", testAdminKey, 100, 100)
	resp, raw := e.do(t, http.MethodGet, "/health", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health status = %d", resp.StatusCode)
	}
	var health struct {
		Status   string `json:"status"`
		Database string `json:"database"`
		Version  string `json:"version"`
	}
	if err := json.Unmarshal(raw, &health); err != nil {
		t.Fatalf("decode health: %v", err)
	}
	if health.Status != "ok" || health.Database != "up" {
		t.Fatalf("unexpected health: %+v", health)
	}
	if strings.Contains(string(raw), testAdminKey) {
		t.Fatal("health must not leak secrets")
	}
}

func TestAdminAuthRequired(t *testing.T) {
	e := newTestEnv(t, "httpapi_auth", testAdminKey, 100, 100)
	resp, _ := e.do(t, http.MethodGet, "/api/v1/admin/instances", "", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("missing key status = %d", resp.StatusCode)
	}
	resp, _ = e.do(t, http.MethodGet, "/api/v1/admin/instances", "wrong-key", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong key status = %d", resp.StatusCode)
	}
	resp, _ = e.do(t, http.MethodGet, "/api/v1/admin/instances", testAdminKey, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("valid key status = %d", resp.StatusCode)
	}
}

func TestCreateListDeleteInstance(t *testing.T) {
	e := newTestEnv(t, "httpapi_crud", testAdminKey, 100, 100)
	created := createInstance(t, e, "shop_101", "https://example.com/hooks")
	if created["api_key"] == nil || created["api_key"] == "" {
		t.Fatal("api_key must be returned once")
	}
	if created["webhook_secret"] == nil || created["webhook_secret"] == "" {
		t.Fatal("webhook_secret must be returned once")
	}
	apiKey := created["api_key"].(string)
	secret := created["webhook_secret"].(string)
	id := created["id"].(string)

	if !strings.HasPrefix(apiKey, auth.ShopKeyPrefix) || !strings.HasPrefix(secret, "whsec_") {
		t.Fatalf("unexpected key formats: %q %q", apiKey, secret)
	}

	resp, raw := e.do(t, http.MethodGet, "/api/v1/admin/instances", testAdminKey, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d", resp.StatusCode)
	}
	if strings.Contains(string(raw), apiKey) || strings.Contains(string(raw), secret) {
		t.Fatal("list must never expose plaintext secrets")
	}
	if strings.Contains(string(raw), "wa_jid") {
		t.Fatal("list must not expose whatsapp session identifiers")
	}

	resp, _ = e.do(t, http.MethodGet, "/api/v1/admin/instances/"+id, testAdminKey, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get status = %d", resp.StatusCode)
	}

	resp, raw = e.do(t, http.MethodPost, "/api/v1/admin/instances", testAdminKey, map[string]any{
		"shop_id": "shop_101", "display_name": "مكرر",
	})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate shop_id must be rejected with 409, got %d body=%s", resp.StatusCode, raw)
	}

	resp, _ = e.do(t, http.MethodDelete, "/api/v1/admin/instances/"+id, testAdminKey, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d", resp.StatusCode)
	}
	resp, _ = e.do(t, http.MethodGet, "/api/v1/admin/instances/"+id, testAdminKey, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("get after delete status = %d", resp.StatusCode)
	}
}

func TestDuplicateShopIDRejected(t *testing.T) {
	e := newTestEnv(t, "httpapi_dup", testAdminKey, 100, 100)
	createInstance(t, e, "shop_300", "")
	resp, raw := e.do(t, http.MethodPost, "/api/v1/admin/instances", testAdminKey, map[string]any{
		"shop_id": "shop_300", "display_name": "مكرر",
	})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate shop_id must return 409, got %d body=%s", resp.StatusCode, raw)
	}
}

func TestCreateInstanceValidation(t *testing.T) {
	e := newTestEnv(t, "httpapi_create_validation", testAdminKey, 100, 100)
	cases := []map[string]any{
		{"shop_id": "bad shop id!", "display_name": "x"},
		{"shop_id": "shop_ok", "display_name": ""},
		{"shop_id": "shop_ok", "display_name": "x", "webhook_url": "ftp://example.com"},
		{"shop_id": "shop_ok", "display_name": "x", "webhook_url": "http://127.0.0.1/hook"},
		{"shop_id": "shop_ok", "display_name": "x", "extra_field": true},
	}
	for _, body := range cases {
		resp, raw := e.do(t, http.MethodPost, "/api/v1/admin/instances", testAdminKey, body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("body %v: expected 400, got %d (%s)", body, resp.StatusCode, raw)
		}
	}
}

func TestShopAPIKeyRotationAndIsolation(t *testing.T) {
	e := newTestEnv(t, "httpapi_isolation", testAdminKey, 100, 100)
	a := createInstance(t, e, "shop_a", "")
	b := createInstance(t, e, "shop_b", "")
	keyA := a["api_key"].(string)
	keyB := b["api_key"].(string)
	idA := a["id"].(string)

	resp, _ := e.do(t, http.MethodPost, "/api/v1/messages/text", keyA, map[string]any{
		"to": "201001234567", "text": "مرحبا",
	})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("disconnected instance should return 409, got %d", resp.StatusCode)
	}

	instB, err := e.keys.FindInstanceByHash(t.Context(), auth.HashKey(keyB))
	if err != nil || instB.ShopID != "shop_b" {
		t.Fatalf("key B must resolve to shop_b: %v %v", instB, err)
	}

	resp, raw := e.do(t, http.MethodPost, "/api/v1/admin/instances/"+idA+"/api-key/rotate", testAdminKey, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rotate status = %d body=%s", resp.StatusCode, raw)
	}
	rotated := decode[map[string]any](t, raw)
	newKey := rotated["api_key"].(string)
	if newKey == keyA {
		t.Fatal("rotated key must differ")
	}

	resp, _ = e.do(t, http.MethodPost, "/api/v1/messages/text", keyA, map[string]any{
		"to": "201001234567", "text": "مرحبا",
	})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old key must be rejected, got %d", resp.StatusCode)
	}

	resp, _ = e.do(t, http.MethodPost, "/api/v1/messages/text", newKey, map[string]any{
		"to": "201001234567", "text": "مرحبا",
	})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("new key should authenticate and reach the disconnected instance, got %d", resp.StatusCode)
	}

	instB, err = e.keys.FindInstanceByHash(t.Context(), auth.HashKey(keyB))
	if err != nil || instB.ShopID != "shop_b" {
		t.Fatalf("rotating A must not affect B: %v %v", instB, err)
	}
}

func TestSendTextValidation(t *testing.T) {
	e := newTestEnv(t, "httpapi_send", testAdminKey, 100, 100)
	created := createInstance(t, e, "shop_send", "")
	key := created["api_key"].(string)

	cases := []struct {
		name string
		body map[string]any
		want int
	}{
		{"invalid phone", map[string]any{"to": "abc", "text": "hi"}, http.StatusBadRequest},
		{"short phone", map[string]any{"to": "12345", "text": "hi"}, http.StatusBadRequest},
		{"empty text", map[string]any{"to": "201001234567", "text": "  "}, http.StatusBadRequest},
		{"unknown field", map[string]any{"to": "201001234567", "text": "hi", "instance_id": "other"}, http.StatusBadRequest},
		{"disconnected instance", map[string]any{"to": "201001234567", "text": "hi"}, http.StatusConflict},
	}
	for _, tc := range cases {
		resp, raw := e.do(t, http.MethodPost, "/api/v1/messages/text", key, tc.body)
		if resp.StatusCode != tc.want {
			t.Fatalf("%s: status = %d want %d body=%s", tc.name, resp.StatusCode, tc.want, raw)
		}
	}
}

func TestNoShopKeyFails(t *testing.T) {
	e := newTestEnv(t, "httpapi_nokey", testAdminKey, 100, 100)
	resp, _ := e.do(t, http.MethodPost, "/api/v1/messages/text", "", map[string]any{
		"to": "201001234567", "text": "hi",
	})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("missing shop key status = %d", resp.StatusCode)
	}
	resp, _ = e.do(t, http.MethodPost, "/api/v1/messages/text", testAdminKey, map[string]any{
		"to": "201001234567", "text": "hi",
	})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("admin key must not authenticate shop endpoints, got %d", resp.StatusCode)
	}
}

func TestWebhookUpdateRotatesSecret(t *testing.T) {
	e := newTestEnv(t, "httpapi_webhook", testAdminKey, 100, 100)
	created := createInstance(t, e, "shop_hook", "https://example.com/hooks")
	id := created["id"].(string)

	resp, raw := e.do(t, http.MethodPut, "/api/v1/admin/instances/"+id+"/webhook", testAdminKey, map[string]any{
		"webhook_url":           "https://example.com/new-hooks",
		"webhook_enabled":       true,
		"rotate_webhook_secret": true,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("webhook update status = %d body=%s", resp.StatusCode, raw)
	}
	updated := decode[map[string]any](t, raw)
	if updated["webhook_secret"] == nil || updated["webhook_secret"] == "" {
		t.Fatal("rotating must return the new secret once")
	}

	resp, raw = e.do(t, http.MethodPut, "/api/v1/admin/instances/"+id+"/webhook", testAdminKey, map[string]any{
		"webhook_url":     "http://192.168.1.1/hook",
		"webhook_enabled": true,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("private webhook url must be rejected, got %d body=%s", resp.StatusCode, raw)
	}

	resp, raw = e.do(t, http.MethodPut, "/api/v1/admin/instances/"+id+"/webhook", testAdminKey, map[string]any{
		"webhook_url":     "https://example.com/hooks",
		"webhook_enabled": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("disable webhook status = %d body=%s", resp.StatusCode, raw)
	}
	if strings.Contains(string(raw), "webhook_secret\":\"") {
		t.Fatal("secret must not be re-returned when not rotated")
	}
}

func TestStatsEndpoint(t *testing.T) {
	e := newTestEnv(t, "httpapi_stats", testAdminKey, 100, 100)
	createInstance(t, e, "shop_stats_1", "")
	createInstance(t, e, "shop_stats_2", "")
	resp, raw := e.do(t, http.MethodGet, "/api/v1/admin/stats", testAdminKey, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stats status = %d", resp.StatusCode)
	}
	var stats struct {
		Total    int            `json:"total"`
		ByStatus map[string]int `json:"by_status"`
	}
	if err := json.Unmarshal(raw, &stats); err != nil {
		t.Fatalf("decode stats: %v", err)
	}
	if stats.Total != 2 {
		t.Fatalf("stats total = %d", stats.Total)
	}
	if stats.ByStatus[database.StatusDisconnected] != 2 {
		t.Fatalf("expected 2 disconnected, got %+v", stats.ByStatus)
	}
}

func TestRateLimiting(t *testing.T) {
	e := newTestEnv(t, "httpapi_ratelimit", testAdminKey, 0.001, 1)
	created := createInstance(t, e, "shop_rate", "")
	key := created["api_key"].(string)

	first, _ := e.do(t, http.MethodPost, "/api/v1/messages/text", key, map[string]any{
		"to": "201001234567", "text": "hi",
	})
	if first.StatusCode == http.StatusTooManyRequests {
		t.Fatal("first request must not be rate limited")
	}
	second, _ := e.do(t, http.MethodPost, "/api/v1/messages/text", key, map[string]any{
		"to": "201001234567", "text": "hi",
	})
	if second.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("second request must be rate limited, got %d", second.StatusCode)
	}
}

func TestUnknownRoutesAndMethods(t *testing.T) {
	e := newTestEnv(t, "httpapi_routes", testAdminKey, 100, 100)
	resp, _ := e.do(t, http.MethodGet, "/api/v1/admin/nope", testAdminKey, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown api route status = %d", resp.StatusCode)
	}
	resp, _ = e.do(t, http.MethodDelete, "/api/v1/messages/text", "", nil)
	if resp.StatusCode != http.StatusMethodNotAllowed && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unexpected status for wrong method: %d", resp.StatusCode)
	}
}

func TestOpenAPISpecServed(t *testing.T) {
	e := newTestEnv(t, "httpapi_spec", testAdminKey, 100, 100)
	resp, raw := e.do(t, http.MethodGet, "/api/v1/openapi.yaml", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("openapi status = %d", resp.StatusCode)
	}
	if !strings.Contains(string(raw), "openapi: 3.0.3") {
		t.Fatalf("unexpected spec: %s", string(raw[:min(len(raw), 80)]))
	}
}
