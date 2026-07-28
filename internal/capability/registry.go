package capability

import (
	"fmt"
	"sync"
	"time"
)

// Registry is the local Capability Registry (EdgeX Capability Runtime side).
type Registry struct {
	mu           sync.RWMutex
	agentID      string
	agent        Agent
	capabilities map[string]Capability
}

// NewRegistry creates an empty registry for the given agent.
func NewRegistry(agent Agent) *Registry {
	if agent.Version == "" {
		agent.Version = RuntimeVersion
	}
	if agent.Kind == "" {
		agent.Kind = AgentKindDevice
	}
	if agent.Status == "" {
		agent.Status = AgentStatusOnline
	}
	if agent.HeartbeatIntervalSec <= 0 {
		agent.HeartbeatIntervalSec = 30
	}
	return &Registry{
		agentID:      agent.ID,
		agent:        agent,
		capabilities: make(map[string]Capability),
	}
}

// AgentID returns the bound agent id.
func (r *Registry) AgentID() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.agentID
}

// SetAgent updates the agent descriptor fields (capabilities list is managed separately).
func (r *Registry) SetAgent(agent Agent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	agent.ID = r.agentID
	r.agent = agent
}

// GetAgent returns a copy of the current agent descriptor (without embedding all caps).
func (r *Registry) GetAgent() Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a := r.agent
	a.Capabilities = nil
	return a
}

// Register adds or replaces a capability.
func (r *Registry) Register(cap Capability) error {
	if cap.ID == "" {
		return fmt.Errorf("capability id is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if cap.AgentID == "" {
		cap.AgentID = r.agentID
	}
	if cap.TimeoutSec <= 0 {
		cap.TimeoutSec = 10
	}
	r.capabilities[cap.ID] = cap
	return nil
}

// RegisterAll registers multiple capabilities.
func (r *Registry) RegisterAll(caps []Capability) error {
	for _, cap := range caps {
		if err := r.Register(cap); err != nil {
			return err
		}
	}
	return nil
}

// Unregister removes a capability by id.
func (r *Registry) Unregister(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.capabilities, id)
}

// Get returns a capability by id.
func (r *Registry) Get(id string) (Capability, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cap, ok := r.capabilities[id]
	return cap, ok
}

// List returns all registered capabilities.
func (r *Registry) List() []Capability {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Capability, 0, len(r.capabilities))
	for _, cap := range r.capabilities {
		out = append(out, cap)
	}
	return out
}

// ListByCategory filters capabilities by category.
func (r *Registry) ListByCategory(category CapabilityCategory) []Capability {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Capability, 0)
	for _, cap := range r.capabilities {
		if cap.Category == category {
			out = append(out, cap)
		}
	}
	return out
}

// IDs returns all capability ids.
func (r *Registry) IDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.capabilities))
	for id := range r.capabilities {
		out = append(out, id)
	}
	return out
}

// Count returns the number of registered capabilities.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.capabilities)
}

// Snapshot builds a registry snapshot for discovery/index consumers.
func (r *Registry) Snapshot() RegistrySnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.capabilities))
	for id := range r.capabilities {
		ids = append(ids, id)
	}
	return RegistrySnapshot{
		AgentID:           r.agentID,
		LastSeen:          time.Now().UnixMilli(),
		CapabilitiesCount: len(r.capabilities),
		Capabilities:      ids,
		Status:            string(r.agent.Status),
		Version:           r.agent.Version,
	}
}

// SetStatus updates agent online status.
func (r *Registry) SetStatus(status AgentStatus) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agent.Status = status
}
