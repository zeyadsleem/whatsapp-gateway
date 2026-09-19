package whatsapp

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/store/sqlstore"
)

type InstanceInfo struct {
	ID          string
	ShopID      string
	DisplayName string
	WAJID       string
}

type ManagerOptions struct {
	Container    *sqlstore.Container
	Repo         Repository
	Notifier     Notifier
	Log          *slog.Logger
	SendTimeout  time.Duration
	MaxInstances int
}

type Manager struct {
	ctx          context.Context
	cancel       context.CancelFunc
	container    *sqlstore.Container
	repo         Repository
	notifier     Notifier
	log          *slog.Logger
	sendTimeout  time.Duration
	maxInstances int

	wg sync.WaitGroup
	mu sync.RWMutex
	m  map[string]*Client
}

func NewManager(opts ManagerOptions) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		ctx:          ctx,
		cancel:       cancel,
		container:    opts.Container,
		repo:         opts.Repo,
		notifier:     opts.Notifier,
		log:          opts.Log,
		sendTimeout:  opts.SendTimeout,
		maxInstances: opts.MaxInstances,
		m:            make(map[string]*Client),
	}
}

func (m *Manager) Register(info InstanceInfo) (*Client, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.m[info.ID]; ok {
		return existing, nil
	}
	if len(m.m) >= m.maxInstances {
		return nil, ErrMaxInstances
	}
	client, err := NewClient(ClientOptions{
		ID:          info.ID,
		ShopID:      info.ShopID,
		DisplayName: info.DisplayName,
		RestoreJID:  info.WAJID,
		Container:   m.container,
		Repo:        m.repo,
		Notifier:    m.notifier,
		Log:         m.log,
		SendTimeout: m.sendTimeout,
		Parent:      m.ctx,
		WG:          &m.wg,
	})
	if err != nil {
		return nil, err
	}
	m.m[info.ID] = client
	return client, nil
}

func (m *Manager) Get(id string) (*Client, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.m[id]
	return c, ok
}

func (m *Manager) List() []*Client {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Client, 0, len(m.m))
	for _, c := range m.m {
		out = append(out, c)
	}
	return out
}

func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.m)
}

func (m *Manager) Remove(id string) {
	m.mu.Lock()
	delete(m.m, id)
	m.mu.Unlock()
}

func (m *Manager) Statuses() map[string]Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]Status, len(m.m))
	for id, c := range m.m {
		out[id] = c.Snapshot()
	}
	return out
}

func (m *Manager) RestoreAll(ctx context.Context, instances []InstanceInfo) {
	for _, info := range instances {
		if ctx.Err() != nil {
			return
		}
		client, err := m.Register(info)
		if err != nil {
			m.log.Error("failed to restore instance", "instance", info.ShopID, "event", "restore", "result", "failure", "error", err)
			continue
		}
		if err := client.Connect(ctx); err != nil {
			m.log.Warn("failed to start restored instance", "instance", info.ShopID, "event", "restore", "result", "failure", "error", err)
			continue
		}
		client.setStatus(StatusConnecting, nil)
		m.log.Info("restored instance session, reconnecting", "instance", info.ShopID, "event", "restore", "result", "success")
	}
}

func (m *Manager) Shutdown(timeout time.Duration) {
	m.cancel()
	for _, c := range m.List() {
		c.Shutdown()
	}
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		m.log.Warn("background workers did not stop in time", "event", "shutdown", "result", "timeout")
	}
}

func (m *Manager) MaxInstances() int {
	return m.maxInstances
}
