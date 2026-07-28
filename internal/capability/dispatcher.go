package capability

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Handler executes an Invoke for a registered Capability.
type Handler func(ctx context.Context, req InvokeRequest, cap Capability) (any, error)

// Mapper maps a Capability Invoke to a lower-level driver/system command.
// Implementations live in internal/execution; this interface keeps the
// Capability Runtime decoupled from ScanEngine / Driver wiring.
type Mapper interface {
	MapAndExecute(ctx context.Context, req InvokeRequest, cap Capability) (any, error)
}

// Dispatcher is the unified Capability Invoke entry point.
type Dispatcher struct {
	registry *Registry
	mapper   Mapper

	mu       sync.RWMutex
	handlers map[CapabilityCategory]Handler

	invokesMu sync.RWMutex
	invokes   map[string]*invokeRecord
}

type invokeRecord struct {
	Request   InvokeRequest
	Response  InvokeResponse
	UpdatedAt time.Time
}

// NewDispatcher creates an Invoke Dispatcher bound to a registry.
func NewDispatcher(registry *Registry) *Dispatcher {
	return &Dispatcher{
		registry: registry,
		handlers: make(map[CapabilityCategory]Handler),
		invokes:  make(map[string]*invokeRecord),
	}
}

// SetMapper installs the Capability → Driver execution mapper.
func (d *Dispatcher) SetMapper(mapper Mapper) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.mapper = mapper
}

// RegisterHandler registers a category-level handler (device/ai/system/...).
func (d *Dispatcher) RegisterHandler(category CapabilityCategory, handler Handler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers[category] = handler
}

// Dispatch validates and executes an InvokeRequest.
func (d *Dispatcher) Dispatch(ctx context.Context, req InvokeRequest) InvokeResponse {
	start := time.Now()
	if req.InvokeID == "" {
		req.InvokeID = NewInvokeID()
	}
	if req.Target == "" {
		req.Target = d.registry.AgentID()
	}

	resp := InvokeResponse{
		InvokeID: req.InvokeID,
		Status:   InvokeQueued,
		Result:   InvokeResult{Timestamp: NowMilli()},
	}
	d.store(req, resp)

	if req.Capability == "" {
		resp.Status = InvokeRejected
		resp.Result = InvokeResult{
			Success:   false,
			Error:     "capability is required",
			ErrorCode: "E012",
			Timestamp: NowMilli(),
		}
		resp.LatencyMs = time.Since(start).Milliseconds()
		d.store(req, resp)
		return resp
	}

	cap, ok := d.registry.Get(req.Capability)
	if !ok {
		resp.Status = InvokeRejected
		resp.Result = InvokeResult{
			Success:   false,
			Error:     fmt.Sprintf("capability not found: %s", req.Capability),
			ErrorCode: "E009",
			Timestamp: NowMilli(),
		}
		resp.LatencyMs = time.Since(start).Milliseconds()
		d.store(req, resp)
		return resp
	}

	if req.Target != "" && req.Target != d.registry.AgentID() {
		resp.Status = InvokeRejected
		resp.Result = InvokeResult{
			Success:   false,
			Error:     fmt.Sprintf("target agent mismatch: %s", req.Target),
			ErrorCode: "E010",
			Timestamp: NowMilli(),
		}
		resp.LatencyMs = time.Since(start).Milliseconds()
		d.store(req, resp)
		return resp
	}

	timeout := time.Duration(cap.TimeoutSec) * time.Second
	if req.Options.TimeoutSec > 0 {
		timeout = time.Duration(req.Options.TimeoutSec) * time.Second
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resp.Status = InvokeRunning
	d.store(req, resp)

	result, err := d.execute(runCtx, req, cap)
	resp.LatencyMs = time.Since(start).Milliseconds()

	if runCtx.Err() == context.DeadlineExceeded {
		resp.Status = InvokeTimeout
		resp.Result = InvokeResult{
			Success:   false,
			Error:     "invoke timed out",
			ErrorCode: "E003",
			Timestamp: NowMilli(),
		}
		d.store(req, resp)
		return resp
	}

	if err != nil {
		resp.Status = InvokeFailed
		resp.Result = InvokeResult{
			Success:   false,
			Error:     err.Error(),
			ErrorCode: "E001",
			Timestamp: NowMilli(),
		}
		d.store(req, resp)
		return resp
	}

	resp.Status = InvokeCompleted
	resp.Result = InvokeResult{
		Success:   true,
		Values:    result,
		Timestamp: NowMilli(),
	}
	d.store(req, resp)
	return resp
}

func (d *Dispatcher) execute(ctx context.Context, req InvokeRequest, cap Capability) (any, error) {
	d.mu.RLock()
	handler := d.handlers[cap.Category]
	mapper := d.mapper
	d.mu.RUnlock()

	if handler != nil {
		return handler(ctx, req, cap)
	}
	if mapper != nil {
		return mapper.MapAndExecute(ctx, req, cap)
	}
	return nil, fmt.Errorf("no handler registered for category %q (capability %s)", cap.Category, cap.ID)
}

// GetStatus returns the latest invoke record.
func (d *Dispatcher) GetStatus(invokeID string) (InvokeResponse, bool) {
	d.invokesMu.RLock()
	defer d.invokesMu.RUnlock()
	rec, ok := d.invokes[invokeID]
	if !ok {
		return InvokeResponse{}, false
	}
	return rec.Response, true
}

func (d *Dispatcher) store(req InvokeRequest, resp InvokeResponse) {
	d.invokesMu.Lock()
	defer d.invokesMu.Unlock()
	d.invokes[req.InvokeID] = &invokeRecord{
		Request:   req,
		Response:  resp,
		UpdatedAt: time.Now(),
	}
}
