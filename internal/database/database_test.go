package database_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zeyad/whatsapp-gateway/internal/database"
	"github.com/zeyad/whatsapp-gateway/internal/testutil"
)

func TestMigrationsCreateAppSchema(t *testing.T) {
	tdb := testutil.RequireDB(t, "database")
	ctx := context.Background()

	var count int
	err := tdb.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables
		WHERE table_schema = 'app' AND table_name IN ('instances','api_keys','schema_migrations')`).Scan(&count)
	if err != nil {
		t.Fatalf("query information_schema: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 app tables, found %d", count)
	}

	if err := database.Migrate(ctx, tdb.DB.Pool); err != nil {
		t.Fatalf("migrations must be idempotent: %v", err)
	}
}

func TestInstanceRepoCRUD(t *testing.T) {
	tdb := testutil.RequireDB(t, "database_repo")
	repo := database.NewInstanceRepo(tdb.DB)
	ctx := context.Background()

	webhookURL := "https://example.com/hooks"
	inst := &database.Instance{
		ID:               "11111111-1111-1111-1111-111111111111",
		ShopID:           "shop_101",
		DisplayName:      "فرع التجمع",
		Status:           database.StatusDisconnected,
		WebhookURL:       &webhookURL,
		WebhookSecretEnc: []byte{1, 2, 3},
		WebhookEnabled:   true,
	}
	if err := repo.Create(ctx, inst); err != nil {
		t.Fatalf("Create: %v", err)
	}

	dup := &database.Instance{
		ID:          "22222222-2222-2222-2222-222222222222",
		ShopID:      "shop_101",
		DisplayName: "مكرر",
		Status:      database.StatusDisconnected,
	}
	if err := repo.Create(ctx, dup); err == nil {
		t.Fatal("duplicate shop_id must be rejected")
	}

	got, err := repo.GetByID(ctx, inst.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ShopID != "shop_101" || got.DisplayName != "فرع التجمع" || !got.WebhookEnabled {
		t.Fatalf("unexpected instance: %+v", got)
	}
	if got.WAJID != nil || got.PhoneNumber != nil {
		t.Fatal("new instance must not have identity")
	}

	byShop, err := repo.GetByShopID(ctx, "shop_101")
	if err != nil || byShop.ID != inst.ID {
		t.Fatalf("GetByShopID: %v %v", byShop, err)
	}

	if err := repo.SetIdentity(ctx, inst.ID, "201001234567@s.whatsapp.net", "201001234567"); err != nil {
		t.Fatalf("SetIdentity: %v", err)
	}
	if err := repo.MarkConnected(ctx, inst.ID, time.Now()); err != nil {
		t.Fatalf("MarkConnected: %v", err)
	}
	got, _ = repo.GetByID(ctx, inst.ID)
	if got.Status != database.StatusConnected || got.LastConnectedAt == nil || got.PhoneNumber == nil || *got.PhoneNumber != "201001234567" {
		t.Fatalf("connected state not persisted: %+v", got)
	}

	restorable, err := repo.ListRestorable(ctx)
	if err != nil {
		t.Fatalf("ListRestorable: %v", err)
	}
	if len(restorable) != 1 {
		t.Fatalf("expected 1 restorable instance, got %d", len(restorable))
	}

	if err := repo.UpdateStatus(ctx, inst.ID, database.StatusLoggedOut); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	restorable, _ = repo.ListRestorable(ctx)
	if len(restorable) != 0 {
		t.Fatal("logged_out instances must not be restorable")
	}

	if err := repo.UpdateMeta(ctx, inst.ID, "اسم جديد", "https://example.com/new", []byte{9}, false); err != nil {
		t.Fatalf("UpdateMeta: %v", err)
	}
	got, _ = repo.GetByID(ctx, inst.ID)
	if got.DisplayName != "اسم جديد" || got.WebhookEnabled || got.WebhookURL == nil || *got.WebhookURL != "https://example.com/new" {
		t.Fatalf("meta not updated: %+v", got)
	}

	if err := repo.Delete(ctx, inst.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.GetByID(ctx, inst.ID); !errors.Is(err, database.ErrNotFound) {
		t.Fatalf("expected not found after delete, got %v", err)
	}
}

func TestAPIKeyRepo(t *testing.T) {
	tdb := testutil.RequireDB(t, "database_keys")
	repo := database.NewInstanceRepo(tdb.DB)
	keys := database.NewAPIKeyRepo(tdb.DB)
	ctx := context.Background()

	inst := &database.Instance{
		ID:          "33333333-3333-3333-3333-333333333333",
		ShopID:      "shop_500",
		DisplayName: "متجر",
		Status:      database.StatusDisconnected,
	}
	if err := repo.Create(ctx, inst); err != nil {
		t.Fatalf("Create: %v", err)
	}

	key := &database.APIKey{
		ID:         "44444444-4444-4444-4444-444444444444",
		InstanceID: inst.ID,
		KeyHash:    strings.Repeat("a", 64),
		KeyPrefix:  "wgs_abcd",
	}
	if err := keys.Upsert(ctx, key); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	found, err := keys.FindInstanceByHash(ctx, key.KeyHash)
	if err != nil {
		t.Fatalf("FindInstanceByHash: %v", err)
	}
	if found.ID != inst.ID {
		t.Fatalf("wrong instance resolved: %s", found.ID)
	}

	if err := keys.TouchLastUsed(ctx, key.KeyHash); err != nil {
		t.Fatalf("TouchLastUsed: %v", err)
	}
	info, err := keys.GetByInstanceID(ctx, inst.ID)
	if err != nil {
		t.Fatalf("GetByInstanceID: %v", err)
	}
	if info.LastUsedAt == nil {
		t.Fatal("last_used_at must be set")
	}

	rotated := &database.APIKey{
		ID:         "55555555-5555-5555-5555-555555555555",
		InstanceID: inst.ID,
		KeyHash:    strings.Repeat("b", 64),
		KeyPrefix:  "wgs_efgh",
	}
	if err := keys.Upsert(ctx, rotated); err != nil {
		t.Fatalf("rotate Upsert: %v", err)
	}
	if _, err := keys.FindInstanceByHash(ctx, key.KeyHash); !errors.Is(err, database.ErrNotFound) {
		t.Fatal("old key must stop working after rotation")
	}

	if _, err := keys.FindInstanceByHash(ctx, strings.Repeat("c", 64)); !errors.Is(err, database.ErrNotFound) {
		t.Fatalf("unknown key must not resolve, got %v", err)
	}
}

// TestPrivacySchemaAudit proves that no application table can hold message,
// chat or contact content.
func TestPrivacySchemaAudit(t *testing.T) {
	tdb := testutil.RequireDB(t, "database_privacy")
	ctx := context.Background()

	forbiddenTables := []string{"messages", "message", "chats", "chat", "conversations",
		"conversation", "contacts", "contact", "message_history", "media", "inbox"}

	rows, err := tdb.DB.Pool.Query(ctx, `SELECT table_name FROM information_schema.tables WHERE table_schema = 'app'`)
	if err != nil {
		t.Fatalf("query tables: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		for _, forbidden := range forbiddenTables {
			if name == forbidden {
				t.Fatalf("forbidden application table found: %s", name)
			}
		}
	}

	forbiddenColumns := []string{"message_text", "text", "body", "content", "caption", "chat_id", "contact_id"}
	rows, err = tdb.DB.Pool.Query(ctx, `SELECT table_name, column_name FROM information_schema.columns
		WHERE table_schema = 'app'`)
	if err != nil {
		t.Fatalf("query columns: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var table, column string
		if err := rows.Scan(&table, &column); err != nil {
			t.Fatalf("scan: %v", err)
		}
		for _, forbidden := range forbiddenColumns {
			if column == forbidden && table != "schema_migrations" {
				t.Fatalf("forbidden column %s.%s found", table, column)
			}
		}
	}

	// The whatsmeow protocol schema is allowed to exist, but the application
	// schema must never contain its tables.
	rows, err = tdb.DB.Pool.Query(ctx, `SELECT count(*) FROM information_schema.tables
		WHERE table_schema = 'app' AND table_name LIKE 'whatsmeow_%'`)
	if err != nil {
		t.Fatalf("query whatsmeow tables: %v", err)
	}
	defer rows.Close()
	var waTables int
	if rows.Next() {
		if err := rows.Scan(&waTables); err != nil {
			t.Fatalf("scan: %v", err)
		}
	}
	if waTables != 0 {
		t.Fatalf("whatsmeow tables leaked into app schema: %d", waTables)
	}
}
