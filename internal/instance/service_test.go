package instance_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.mau.fi/whatsmeow/proto/waAdv"
	"go.mau.fi/whatsmeow/types"

	"github.com/zeyad/whatsapp-gateway/internal/database"
	"github.com/zeyad/whatsapp-gateway/internal/instance"
	"github.com/zeyad/whatsapp-gateway/internal/message"
	"github.com/zeyad/whatsapp-gateway/internal/testutil"
	"github.com/zeyad/whatsapp-gateway/internal/whatsapp"
)

func TestRestoreRegistersAllInstancesWithoutStartingPairing(t *testing.T) {
	tdb := testutil.RequireDB(t, "instance_restore")
	repo := database.NewInstanceRepo(tdb.DB)
	manager := whatsapp.NewManager(whatsapp.ManagerOptions{
		Container: tdb.Container, Repo: repo, Log: tdb.Log,
		MaxInstances: 20, SendTimeout: time.Second,
	})
	t.Cleanup(func() { manager.Shutdown(time.Second) })
	svc := instance.NewService(repo, database.NewAPIKeyRepo(tdb.DB), manager, nil, tdb.Log)
	statuses := []string{
		database.StatusDisconnected, database.StatusConnecting, database.StatusQRRequired,
		database.StatusConnected, database.StatusReconnecting, database.StatusLoggedOut,
		database.StatusError,
	}
	wantReconnect := make(map[string]bool)
	var ids []string
	for i, status := range statuses {
		for _, paired := range []bool{false, true} {
			inst := &database.Instance{
				ID: uuid.NewString(), ShopID: fmt.Sprintf("shop_%d_%t", i, paired),
				DisplayName: "Test shop", Status: status,
			}
			if err := repo.Create(t.Context(), inst); err != nil {
				t.Fatal(err)
			}
			if paired {
				device := tdb.Container.NewDevice()
				jid := types.NewJID(fmt.Sprintf("20100123456%d", i), types.DefaultUserServer)
				device.ID = &jid
				device.Account = &waAdv.ADVSignedDeviceIdentity{
					Details: []byte{}, AccountSignature: make([]byte, 64),
					AccountSignatureKey: make([]byte, 32), DeviceSignature: make([]byte, 64),
				}
				if err := device.Save(t.Context()); err != nil {
					t.Fatal(err)
				}
				if err := repo.SetIdentity(t.Context(), inst.ID, jid.String(), jid.User); err != nil {
					t.Fatal(err)
				}
				if status != database.StatusLoggedOut && status != database.StatusError {
					wantReconnect[inst.ID] = true
				}
			}
			ids = append(ids, inst.ID)
		}
	}
	reconnect, err := svc.Restore(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if manager.Count() != len(ids) || len(reconnect) != len(wantReconnect) {
		t.Fatalf("registered=%d reconnect=%d; want %d and %d", manager.Count(), len(reconnect), len(ids), len(wantReconnect))
	}
	for _, info := range reconnect {
		if !wantReconnect[info.ID] || info.WAJID == "" {
			t.Fatalf("unexpected auto-reconnect candidate: %s", info.ID)
		}
		delete(wantReconnect, info.ID)
	}
	msg := message.NewService(manager, 1024, tdb.Log)
	for _, id := range ids {
		client, ok := manager.Get(id)
		if !ok || client.IsConnected() {
			t.Fatalf("instance %s must be registered but offline", id)
		}
		if _, _, err := client.QR(); err == nil {
			t.Fatal("restore must not start QR pairing")
		}
		if err := svc.Disconnect(t.Context(), id); err != nil {
			t.Fatalf("restored lifecycle operation: %v", err)
		}
		_, err := msg.SendText(t.Context(), id, "201001234567", "hi")
		if !errors.Is(err, whatsapp.ErrNotPaired) && !errors.Is(err, whatsapp.ErrNotConnected) {
			t.Fatalf("expected offline instance, not missing instance: %v", err)
		}
	}
}

func TestRestoreReportsMissingSession(t *testing.T) {
	tdb := testutil.RequireDB(t, "instance_missing_session")
	repo := database.NewInstanceRepo(tdb.DB)
	inst := &database.Instance{ID: uuid.NewString(), ShopID: "missing", DisplayName: "Missing", Status: database.StatusConnected}
	if err := repo.Create(t.Context(), inst); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetIdentity(t.Context(), inst.ID, "201001234567@s.whatsapp.net", "201001234567"); err != nil {
		t.Fatal(err)
	}
	manager := whatsapp.NewManager(whatsapp.ManagerOptions{
		Container: tdb.Container, Repo: repo, Log: tdb.Log, MaxInstances: 10,
	})
	t.Cleanup(func() { manager.Shutdown(time.Second) })
	svc := instance.NewService(repo, nil, manager, nil, tdb.Log)
	if _, err := svc.Restore(t.Context()); !errors.Is(err, whatsapp.ErrNoDevice) {
		t.Fatalf("expected missing session error, got %v", err)
	}
	if manager.Count() != 0 {
		t.Fatal("missing session must not silently create a fresh device")
	}
}
