package openfeature

import (
	"context"
	"log"
	"sync"
	"time"
)

// octopusContextProvider manages the cached feature evaluation context and
// handles background refresh of feature toggles from the Octopus service.
type octopusContextProvider struct {
	client        *octopusClient
	cacheDuration time.Duration

	// Thread-safe access to the current context
	mu             sync.RWMutex
	currentContext *octopusContext
	contentHash    []byte

	// Background refresh control
	stopChan chan struct{}
	doneChan chan struct{}
}

// newOctopusContextProvider creates a new context provider with the given client and cache duration.
func newOctopusContextProvider(client *octopusClient, cacheDuration time.Duration) *octopusContextProvider {
	return &octopusContextProvider{
		client:         client,
		cacheDuration:  cacheDuration,
		currentContext: newOctopusContext(nil), // Start with empty context
		stopChan:       make(chan struct{}),
		doneChan:       make(chan struct{}),
	}
}

// initialize fetches the initial feature toggle manifest and starts the background refresh loop.
func (p *octopusContextProvider) initialize(ctx context.Context) error {
	// Fetch initial manifest
	if err := p.refreshContext(); err != nil {
		log.Printf("Failed to fetch initial feature toggles, starting with empty context: %v", err)
		// Continue with empty context - graceful degradation
	}

	// Start background refresh goroutine
	go p.refreshLoop()

	return nil
}

// shutdown gracefully stops the background refresh goroutine.
func (p *octopusContextProvider) shutdown(ctx context.Context) error {
	close(p.stopChan)
	// Wait for refresh goroutine to finish
	select {
	case <-p.doneChan:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// getContext returns the current cached evaluation context in a thread-safe manner.
func (p *octopusContextProvider) getContext() *octopusContext {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.currentContext
}

// refreshLoop continuously checks for feature toggle changes and refreshes the context.
func (p *octopusContextProvider) refreshLoop() {
	defer close(p.doneChan)

	ticker := time.NewTicker(p.cacheDuration)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopChan:
			log.Println("Stopping feature toggle refresh loop")
			return
		case <-ticker.C:
			if err := p.checkAndRefresh(); err != nil {
				log.Printf("Failed to check/refresh feature toggles: %v", err)
				// Continue trying on next tick
			}
		}
	}
}

// checkAndRefresh checks if feature toggles have changed and refreshes if needed.
func (p *octopusContextProvider) checkAndRefresh() error {
	p.mu.RLock()
	currentHash := p.contentHash
	p.mu.RUnlock()

	// Check if toggles have changed
	changed, newHash, err := p.client.haveFeatureTogglesChanged(currentHash)
	if err != nil {
		return err
	}

	// If changed, fetch new manifest
	if changed {
		log.Println("Feature toggles have changed, fetching new manifest")
		if err := p.refreshContext(); err != nil {
			return err
		}
	} else {
		// Update hash even if not changed (in case it was nil before)
		if newHash != nil && currentHash == nil {
			p.mu.Lock()
			p.contentHash = newHash
			p.mu.Unlock()
		}
	}

	return nil
}

// refreshContext fetches the feature toggle manifest and updates the cached context.
func (p *octopusContextProvider) refreshContext() error {
	toggles, err := p.client.getFeatureToggleEvaluationManifest()
	if err != nil {
		return err
	}

	// Update context atomically
	p.mu.Lock()
	p.currentContext = newOctopusContext(toggles)
	p.contentHash = toggles.ContentHash
	p.mu.Unlock()

	log.Printf("Successfully refreshed feature toggles (%d evaluations)", len(toggles.Evaluations))
	return nil
}
