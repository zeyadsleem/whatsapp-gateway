package webhook_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/zeyad/whatsapp-gateway/internal/crypto"
	"github.com/zeyad/whatsapp-gateway/internal/database"
	"github.com/zeyad/whatsapp-gateway/internal/testutil"
	"github.com/zeyad/whatsapp-gateway/internal/webhook"
	"github.com/zeyad/whatsapp-gateway/internal/whatsapp"
)

func appRowCounts(t *testing.T, ctx context.Context, tdb *testutil.TestDB) map[string]int {
	t.Helper()
	rows, err := tdb.DB.Pool.Query(ctx, `SELECT table_name FROM information_schema.tables WHERE table_schema = 'app'`)
	if err != nil {
		t.Fatalf("list app tables: %v", err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table: %v", err)
		}
		tables = append(tables, name)
	}
	rows.Close()

	counts := make(map[string]int, len(tables))
	for _, table := range tables {
		var n int
		if err := tdb.DB.Pool.QueryRow(ctx, fmt.Sprintf("SELECT count(*) FROM app.%s", table)).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		counts[table] = n
	}
	return counts
}

// TestMessageEventDoesNotPersist proves that dispatching an incoming message
// never writes message content or any other row into the application schema.
func TestMessageEventDoesNotPersist(t *testing.T) {
	tdb := testutil.RequireDB(t, "webhook_privacy")
	repo := database.NewInstanceRepo(tdb.DB)
	box, err := crypto.New("test-encryption-key-for-privacy-audit")
	if err != nil {
		t.Fatalf("crypto.New: %v", err)
	}
	dispatcher := webhook.NewDispatcher(webhook.Config{
		Timeout:    time.Second,
		MaxRetries: 1,
		Workers:    2,
		QueueSize:  16,
		Log:        tdb.Log,
	}, repo, box)
	t.Cleanup(func() { dispatcher.Shutdown(time.Second) })

	ctx := context.Background()
	webhookURL := "https://example.com/hooks"
	secret, err := box.Encrypt([]byte("whsec_privacy_test"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	inst := &database.Instance{
		ID:               uuid.NewString(),
		ShopID:           "shop_privacy",
		DisplayName:      "متجر الخصوصية",
		Status:           database.StatusConnected,
		WebhookURL:       &webhookURL,
		WebhookSecretEnc: secret,
		WebhookEnabled:   true,
	}
	if err := repo.Create(ctx, inst); err != nil {
		t.Fatalf("create instance: %v", err)
	}

	before := appRowCounts(t, ctx, tdb)

	dispatcher.MessageEvent(whatsapp.MessageEvent{
		InstanceID: inst.ID,
		ShopID:     inst.ShopID,
		From:       "201001234567",
		MessageID:  "3EB0TESTMESSAGE",
		Type:       "text",
		Text:       "سر للغاية: رسالة عميل يجب ألا تُخزَّن",
		Timestamp:  time.Now(),
		EventID:    uuid.NewString(),
	})
	dispatcher.InstanceEvent(whatsapp.InstanceEvent{
		Type:       whatsapp.EventConnected,
		InstanceID: inst.ID,
		ShopID:     inst.ShopID,
		Phone:      "201001234567",
		Timestamp:  time.Now(),
		EventID:    uuid.NewString(),
	})

	time.Sleep(500 * time.Millisecond)

	after := appRowCounts(t, ctx, tdb)
	for table, count := range before {
		if after[table] != count {
			t.Fatalf("table %s changed from %d to %d rows after message dispatch", table, count, after[table])
		}
	}

	var containsText bool
	err = tdb.DB.Pool.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM app.instances WHERE display_name LIKE '%سر للغاية%'
	)`).Scan(&containsText)
	if err != nil {
		t.Fatalf("scan message text: %v", err)
	}
	if containsText {
		t.Fatal("message text leaked into app.instances")
	}
}
