package client

// ---------- Agent registration ----------

type RegisterRequest struct {
	AgentID string `json:"agent_id"`

	Capacity Capacity `json:"capacity"`
}

type Capacity struct {
	SNI   int `json:"sni"`
	Route int `json:"route"`
}

// ---------- Poll ----------

type PollRequest struct {
	AgentID string `json:"agent_id"`

	Max int `json:"max"`
}

type PollResponse struct {
	Revision uint64 `json:"revision"`

	Leases []Lease `json:"leases"`
}

// ---------- Lease ----------

type Lease struct {
	ID string `json:"id"`

	Domain string `json:"domain"`

	Type string `json:"type"`

	State string `json:"state"`

	AgentID string `json:"agent_id,omitempty"`

	ExpiresAt string `json:"expires_at,omitempty"`
}

// ---------- Heartbeat ----------

type HeartbeatRequest struct {
	AgentID string `json:"agent_id"`

	LeaseIDs []string `json:"lease_ids"`
}

// ---------- Report ----------

type ReportRequest struct {
	AgentID string `json:"agent_id"`

	LeaseID string `json:"lease_id"`

	Success bool `json:"success"`

	Address string `json:"address,omitempty"`

	SNI string `json:"sni,omitempty"`

	RouteID string `json:"route_id,omitempty"`
}
