package whatsapp

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

type identityRepo struct {
	jid   string
	phone string
}

func (*identityRepo) UpdateStatus(context.Context, string, string) error { return nil }
func (r *identityRepo) SetIdentity(_ context.Context, _, jid, phone string) error {
	r.jid, r.phone = jid, phone
	return nil
}
func (*identityRepo) ClearIdentity(context.Context, string) error            { return nil }
func (*identityRepo) MarkConnected(context.Context, string, time.Time) error { return nil }

func TestConnectedPersistsIdentityAfterPairSuccess(t *testing.T) {
	jid := types.NewJID("201001234567", types.DefaultUserServer)
	repo := &identityRepo{}
	wg := &sync.WaitGroup{}
	c := &Client{
		id: "paired-instance", ctx: t.Context(), repo: repo, wg: wg,
		log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		wa:  whatsmeow.NewClient(&store.Device{ID: &jid}, nil),
	}
	c.onPairSuccess(&events.PairSuccess{ID: jid})
	c.onConnected()
	wg.Wait()
	if repo.jid != jid.String() || repo.phone != jid.User {
		t.Fatalf("paired identity not persisted: jid=%q phone=%q", repo.jid, repo.phone)
	}
}
