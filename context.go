package openfeature

import (
	"fmt"
	"strings"

	"github.com/open-feature/go-sdk/openfeature"
)

// octopusContext encapsulates feature evaluation logic with segment matching.
type octopusContext struct {
	toggles *FeatureToggles
}

// newOctopusContext creates a new evaluation context with the given feature toggles.
func newOctopusContext(toggles *FeatureToggles) *octopusContext {
	if toggles == nil {
		toggles = &FeatureToggles{
			Evaluations: []FeatureToggleEvaluation{},
		}
	}
	return &octopusContext{
		toggles: toggles,
	}
}

// evaluate evaluates a feature toggle by its slug and returns the resolution detail.
func (c *octopusContext) evaluate(slug string, defaultValue bool, evalContext openfeature.FlattenedContext) openfeature.BoolResolutionDetail {
	// Find the toggle by slug (case-insensitive)
	toggle := c.findToggle(slug)
	if toggle == nil {
		// Toggle not found - return default value with error
		return openfeature.BoolResolutionDetail{
			Value: defaultValue,
			ProviderResolutionDetail: openfeature.ProviderResolutionDetail{
				ResolutionError: openfeature.NewGeneralResolutionError("flag not found"),
				Reason:          openfeature.ErrorReason,
			},
		}
	}

	// If toggle is disabled, return false
	if !toggle.IsEnabled {
		return openfeature.BoolResolutionDetail{
			Value: false,
			ProviderResolutionDetail: openfeature.ProviderResolutionDetail{
				Reason: openfeature.DefaultReason,
			},
		}
	}

	// If toggle has no segments, return enabled value
	if len(toggle.Segments) == 0 {
		return openfeature.BoolResolutionDetail{
			Value: true,
			ProviderResolutionDetail: openfeature.ProviderResolutionDetail{
				Reason: openfeature.DefaultReason,
			},
		}
	}

	// Evaluate segments against context
	if c.matchesSegment(toggle, evalContext) {
		return openfeature.BoolResolutionDetail{
			Value: true,
			ProviderResolutionDetail: openfeature.ProviderResolutionDetail{
				Reason: openfeature.TargetingMatchReason,
			},
		}
	}

	// Segments don't match - return false
	return openfeature.BoolResolutionDetail{
		Value: false,
		ProviderResolutionDetail: openfeature.ProviderResolutionDetail{
			Reason: openfeature.DefaultReason,
		},
	}
}

// findToggle finds a feature toggle by slug using case-insensitive matching.
func (c *octopusContext) findToggle(slug string) *FeatureToggleEvaluation {
	slugLower := strings.ToLower(slug)
	for i := range c.toggles.Evaluations {
		toggle := &c.toggles.Evaluations[i]
		if strings.ToLower(toggle.Slug) == slugLower {
			return toggle
		}
	}
	return nil
}

// matchesSegment checks if the evaluation context matches the toggle's segments.
// All segment keys must be present in the context, and for each key, at least one
// segment value must match a context value (case-insensitive).
func (c *octopusContext) matchesSegment(toggle *FeatureToggleEvaluation, evalContext openfeature.FlattenedContext) bool {
	if len(toggle.Segments) == 0 {
		return true
	}

	// Build a map of segment keys to their possible values
	segmentMap := make(map[string][]string)
	for _, segment := range toggle.Segments {
		for key, value := range segment {
			keyLower := strings.ToLower(key)
			valueLower := strings.ToLower(value)
			segmentMap[keyLower] = append(segmentMap[keyLower], valueLower)
		}
	}

	// All segment keys must be present and match
	for segmentKey, segmentValues := range segmentMap {
		contextValue, exists := evalContext[segmentKey]
		if !exists {
			// Try case-insensitive key lookup
			exists, contextValue = c.findContextValue(evalContext, segmentKey)
			if !exists {
				return false
			}
		}

		// Check if context value matches any of the segment values
		contextValueStr := c.toString(contextValue)
		contextValueLower := strings.ToLower(contextValueStr)

		matched := false
		for _, segmentValue := range segmentValues {
			if contextValueLower == segmentValue {
				matched = true
				break
			}
		}

		if !matched {
			return false
		}
	}

	return true
}

// findContextValue performs a case-insensitive lookup in the evaluation context.
func (c *octopusContext) findContextValue(evalContext openfeature.FlattenedContext, key string) (bool, interface{}) {
	keyLower := strings.ToLower(key)
	for k, v := range evalContext {
		if strings.ToLower(k) == keyLower {
			return true, v
		}
	}
	return false, nil
}

// toString converts an interface value to a string.
func (c *octopusContext) toString(value interface{}) string {
	if value == nil {
		return ""
	}
	if str, ok := value.(string); ok {
		return str
	}
	// For other types, use fmt.Sprintf
	return fmt.Sprintf("%v", value)
}
