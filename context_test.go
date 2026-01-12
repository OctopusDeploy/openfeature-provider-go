package openfeature

import (
	"testing"

	"github.com/open-feature/go-sdk/openfeature"
)

func TestEvaluate_FeatureEnabled(t *testing.T) {
	toggles := &FeatureToggles{
		Evaluations: []FeatureToggleEvaluation{
			{
				Name:      "Dark Mode",
				Slug:      "dark-mode",
				IsEnabled: true,
				Segments:  []map[string]string{},
			},
		},
	}

	ctx := newOctopusContext(toggles)
	result := ctx.evaluate("dark-mode", false, openfeature.FlattenedContext{})

	if !result.Value {
		t.Errorf("Expected true, got false")
	}
	if result.Reason != openfeature.DefaultReason {
		t.Errorf("Expected DefaultReason, got %v", result.Reason)
	}
}

func TestEvaluate_FeatureDisabled(t *testing.T) {
	toggles := &FeatureToggles{
		Evaluations: []FeatureToggleEvaluation{
			{
				Name:      "Dark Mode",
				Slug:      "dark-mode",
				IsEnabled: false,
				Segments:  []map[string]string{},
			},
		},
	}

	ctx := newOctopusContext(toggles)
	result := ctx.evaluate("dark-mode", true, openfeature.FlattenedContext{})

	if result.Value {
		t.Errorf("Expected false, got true")
	}
	if result.Reason != openfeature.DefaultReason {
		t.Errorf("Expected DefaultReason, got %v", result.Reason)
	}
}

func TestEvaluate_FeatureNotFound(t *testing.T) {
	toggles := &FeatureToggles{
		Evaluations: []FeatureToggleEvaluation{},
	}

	ctx := newOctopusContext(toggles)
	result := ctx.evaluate("non-existent", false, openfeature.FlattenedContext{})

	if result.Value {
		t.Errorf("Expected false (default), got true")
	}
	if result.Reason != openfeature.ErrorReason {
		t.Errorf("Expected ErrorReason, got %v", result.Reason)
	}
	if result.ResolutionError.Error() == "" {
		t.Error("Expected resolution error, got empty")
	}
}

func TestEvaluate_CaseInsensitiveSlug(t *testing.T) {
	toggles := &FeatureToggles{
		Evaluations: []FeatureToggleEvaluation{
			{
				Name:      "Dark Mode",
				Slug:      "dark-mode",
				IsEnabled: true,
				Segments:  []map[string]string{},
			},
		},
	}

	ctx := newOctopusContext(toggles)

	testCases := []string{"dark-mode", "Dark-Mode", "DARK-MODE", "DaRk-MoDe"}
	for _, slug := range testCases {
		result := ctx.evaluate(slug, false, openfeature.FlattenedContext{})
		if !result.Value {
			t.Errorf("Case-insensitive lookup failed for slug: %s", slug)
		}
	}
}

func TestEvaluate_SegmentMatching_SingleSegment(t *testing.T) {
	toggles := &FeatureToggles{
		Evaluations: []FeatureToggleEvaluation{
			{
				Name:      "Premium Feature",
				Slug:      "premium-feature",
				IsEnabled: true,
				Segments: []map[string]string{
					{"license-type": "premium"},
				},
			},
		},
	}

	ctx := newOctopusContext(toggles)

	// Test matching context
	result := ctx.evaluate("premium-feature", false, openfeature.FlattenedContext{
		"license-type": "premium",
	})
	if !result.Value {
		t.Error("Expected true for matching segment")
	}
	if result.Reason != openfeature.TargetingMatchReason {
		t.Errorf("Expected TargetingMatchReason, got %v", result.Reason)
	}

	// Test non-matching context
	result = ctx.evaluate("premium-feature", false, openfeature.FlattenedContext{
		"license-type": "free",
	})
	if result.Value {
		t.Error("Expected false for non-matching segment")
	}
}

func TestEvaluate_SegmentMatching_MultipleSegments(t *testing.T) {
	toggles := &FeatureToggles{
		Evaluations: []FeatureToggleEvaluation{
			{
				Name:      "Regional Feature",
				Slug:      "regional-feature",
				IsEnabled: true,
				Segments: []map[string]string{
					{"license-type": "premium"},
					{"region": "us-east"},
				},
			},
		},
	}

	ctx := newOctopusContext(toggles)

	// Test all segments matching
	result := ctx.evaluate("regional-feature", false, openfeature.FlattenedContext{
		"license-type": "premium",
		"region":       "us-east",
	})
	if !result.Value {
		t.Error("Expected true when all segments match")
	}
	if result.Reason != openfeature.TargetingMatchReason {
		t.Errorf("Expected TargetingMatchReason, got %v", result.Reason)
	}

	// Test missing segment key
	result = ctx.evaluate("regional-feature", false, openfeature.FlattenedContext{
		"license-type": "premium",
	})
	if result.Value {
		t.Error("Expected false when segment key is missing")
	}

	// Test one segment not matching
	result = ctx.evaluate("regional-feature", false, openfeature.FlattenedContext{
		"license-type": "free",
		"region":       "us-east",
	})
	if result.Value {
		t.Error("Expected false when one segment doesn't match")
	}
}

func TestEvaluate_SegmentMatching_ExtraContextValues(t *testing.T) {
	toggles := &FeatureToggles{
		Evaluations: []FeatureToggleEvaluation{
			{
				Name:      "Premium Feature",
				Slug:      "premium-feature",
				IsEnabled: true,
				Segments: []map[string]string{
					{"license-type": "premium"},
				},
			},
		},
	}

	ctx := newOctopusContext(toggles)

	// Extra context values should be ignored
	result := ctx.evaluate("premium-feature", false, openfeature.FlattenedContext{
		"license-type": "premium",
		"extra-key":    "extra-value",
		"another-key":  "another-value",
	})
	if !result.Value {
		t.Error("Expected true - extra context values should be ignored")
	}
}

func TestEvaluate_SegmentMatching_CaseInsensitive(t *testing.T) {
	toggles := &FeatureToggles{
		Evaluations: []FeatureToggleEvaluation{
			{
				Name:      "Premium Feature",
				Slug:      "premium-feature",
				IsEnabled: true,
				Segments: []map[string]string{
					{"license-type": "premium"},
				},
			},
		},
	}

	ctx := newOctopusContext(toggles)

	// Test case-insensitive matching for both keys and values
	result := ctx.evaluate("premium-feature", false, openfeature.FlattenedContext{
		"License-Type": "Premium",
	})
	if !result.Value {
		t.Error("Expected true for case-insensitive segment matching")
	}

	result = ctx.evaluate("premium-feature", false, openfeature.FlattenedContext{
		"LICENSE-TYPE": "PREMIUM",
	})
	if !result.Value {
		t.Error("Expected true for uppercase segment matching")
	}
}

func TestEvaluate_EmptyToggles(t *testing.T) {
	ctx := newOctopusContext(nil)
	result := ctx.evaluate("any-feature", true, openfeature.FlattenedContext{})

	if !result.Value {
		t.Error("Expected true (default value) for empty toggles")
	}
	if result.Reason != openfeature.ErrorReason {
		t.Errorf("Expected ErrorReason, got %v", result.Reason)
	}
}

func TestEvaluate_SegmentMatching_NullContextValue(t *testing.T) {
	toggles := &FeatureToggles{
		Evaluations: []FeatureToggleEvaluation{
			{
				Name:      "Premium Feature",
				Slug:      "premium-feature",
				IsEnabled: true,
				Segments: []map[string]string{
					{"license-type": "premium"},
				},
			},
		},
	}

	ctx := newOctopusContext(toggles)

	// Test with nil context value
	result := ctx.evaluate("premium-feature", false, openfeature.FlattenedContext{
		"license-type": nil,
	})
	if result.Value {
		t.Error("Expected false when context value is nil")
	}
}
