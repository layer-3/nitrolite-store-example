package nitrolite

import "sync"

// Health captures current client lifecycle state.
type Health struct {
	Connected     bool
	Ready         bool
	SignerAddress string
}

// Manager is a temporary lifecycle placeholder until the SDK wiring lands.
type Manager struct {
	mu     sync.RWMutex
	health Health
}

// NewManager constructs a new placeholder manager.
func NewManager(signerAddress string) *Manager {
	return &Manager{
		health: Health{
			Connected:     false,
			Ready:         false,
			SignerAddress: signerAddress,
		},
	}
}

// Health returns a copy of the current lifecycle state.
func (m *Manager) Health() Health {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.health
}

// SetHealth replaces lifecycle state.
func (m *Manager) SetHealth(h Health) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.health = h
}
