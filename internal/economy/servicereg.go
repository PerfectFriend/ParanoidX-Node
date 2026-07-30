// Package economy implements the island economy system
package economy

import (
		"fmt"
		"path/filepath"
	"sync"
	"time"

	"ParanoidX/internal/fileutil"
)

// ServiceType enumerates available service types.
type ServiceType string

const (
	SvcStorage     ServiceType = "storage"
	SvcBanknote    ServiceType = "banknote_press"
	SvcAuction     ServiceType = "auction"
	SvcRadio       ServiceType = "radio"
	SvcAI          ServiceType = "ai_steward"
	SvcAuditor     ServiceType = "auditor"
	SvcMarketplace ServiceType = "marketplace"
)

// RegisteredService describes a service offered by a franchise node.
type RegisteredService struct {
	ID          string      `json:"id"`
	LicenseID   string      `json:"license_id"`
	ServiceType ServiceType `json:"service_type"`
	Name        string      `json:"name"`
	Endpoint    string      `json:"endpoint"`
	Status      string      `json:"status"` // active, degraded, offline
	PricePerUse int64       `json:"price_per_use_ng"`
	LatencyMS   int         `json:"latency_ms,omitempty"`
	RegisteredAt string     `json:"registered_at"`
}

// ServiceRegistry manages the service directory.
type ServiceRegistry struct {
	mu       sync.Mutex
	Services map[string]*RegisteredService `json:"services"`
}

// NewServiceRegistry creates a service registry with an empty service map.
func NewServiceRegistry() *ServiceRegistry {
	return &ServiceRegistry{Services: make(map[string]*RegisteredService)}
}

// LoadServiceRegistry loads the service registry from disk.
func LoadServiceRegistry(dataDir string) *ServiceRegistry {
	sr := NewServiceRegistry()

	fileutil.ReadJSON(filepath.Join(dataDir, "service_registry.json"), sr)
	if sr.Services == nil {
		sr.Services = make(map[string]*RegisteredService)
	}
	return sr
}

// Save persists the service registry to JSON.
func (sr *ServiceRegistry) Save(dataDir string) {
	p := filepath.Join(dataDir, "service_registry.json")
	fileutil.WriteJSON(p, sr)
}

// Register adds or updates a service.
func (sr *ServiceRegistry) Register(svc *RegisteredService) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	if svc.ID == "" {
		return fmt.Errorf("service ID required")
	}
	svc.RegisteredAt = time.Now().UTC().Format(time.RFC3339)
	if svc.Status == "" {
		svc.Status = "active"
	}
	sr.Services[svc.ID] = svc
	return nil
}

// Get retrieves a service by ID.
func (sr *ServiceRegistry) Get(id string) (*RegisteredService, error) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	svc, ok := sr.Services[id]
	if !ok {
		return nil, fmt.Errorf("service %q not found", id)
	}
	return svc, nil
}

// ListByType returns all services of a given type.
func (sr *ServiceRegistry) ListByType(svcType ServiceType) []*RegisteredService {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	var out []*RegisteredService
	for _, svc := range sr.Services {
		if svc.ServiceType == svcType && svc.Status == "active" {
			out = append(out, svc)
		}
	}
	return out
}

// ListAll returns all registered services.
func (sr *ServiceRegistry) ListAll() []*RegisteredService {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	out := make([]*RegisteredService, 0, len(sr.Services))
	for _, svc := range sr.Services {
		out = append(out, svc)
	}
	return out
}

// Deregister removes a service by ID.
func (sr *ServiceRegistry) Deregister(id string) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	if _, ok := sr.Services[id]; !ok {
		return fmt.Errorf("service %q not found", id)
	}
	delete(sr.Services, id)
	return nil
}

// --- Service Marketplace ---

// ServiceListing is a sell offer for a registered service on the marketplace.
type ServiceListing struct {
	ID          string `json:"id"`
	ServiceID   string `json:"service_id"`
	SellerID    string `json:"seller_id"`
	PricePerUse int64  `json:"price_per_use_ng"`
	Description string `json:"description"`
	Available   bool   `json:"available"`
	CreatedAt   string `json:"created_at"`
}

// ServiceMarketplace manages buy/sell listings for franchise services.
type ServiceMarketplace struct {
	mu       sync.Mutex
	Listings map[string]*ServiceListing `json:"listings"`
}

// NewServiceMarketplace creates a marketplace with an empty listings map.
func NewServiceMarketplace() *ServiceMarketplace {
	return &ServiceMarketplace{Listings: make(map[string]*ServiceListing)}
}

// LoadMarketplace loads service marketplace listings from disk.
func LoadMarketplace(dataDir string) *ServiceMarketplace {
	sm := NewServiceMarketplace()

	fileutil.ReadJSON(filepath.Join(dataDir, "service_marketplace.json"), sm)
	if sm.Listings == nil {
		sm.Listings = make(map[string]*ServiceListing)
	}
	return sm
}

// Save persists marketplace listings to JSON.
func (sm *ServiceMarketplace) Save(dataDir string) {
	p := filepath.Join(dataDir, "service_marketplace.json")
	fileutil.WriteJSON(p, sm)
}

// List creates a new sell listing for a service on the marketplace.
func (sm *ServiceMarketplace) List(serviceID, sellerID string) (*ServiceListing, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	id := fmt.Sprintf("list-%s-%s", sellerID, serviceID)
	if _, exists := sm.Listings[id]; exists {
		return nil, fmt.Errorf("listing %q already exists", id)
	}
	listing := &ServiceListing{
		ID:        id,
		ServiceID: serviceID,
		SellerID:  sellerID,
		Available: true,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	sm.Listings[id] = listing
	return listing, nil
}

// Buy marks a service listing as no longer available (purchased).
func (sm *ServiceMarketplace) Buy(id string) (*ServiceListing, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	listing, ok := sm.Listings[id]
	if !ok {
		return nil, fmt.Errorf("listing %q not found", id)
	}
	if !listing.Available {
		return nil, fmt.Errorf("listing %q is no longer available", id)
	}
	listing.Available = false
	return listing, nil
}

// Remove deletes a service listing by ID.
func (sm *ServiceMarketplace) Remove(id string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, ok := sm.Listings[id]; !ok {
		return fmt.Errorf("listing %q not found", id)
	}
	delete(sm.Listings, id)
	return nil
}

// Search returns all available marketplace listings.
func (sm *ServiceMarketplace) Search(svcType ServiceType) []*ServiceListing {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	var out []*ServiceListing
	for _, l := range sm.Listings {
		if l.Available {
			out = append(out, l)
		}
	}
	return out
}
