package openfeature

// FeatureToggles represents the collection of feature toggle evaluations
// returned from the Octopus Deploy feature toggle service.
type FeatureToggles struct {
	// Evaluations contains the list of individual feature toggle evaluations
	Evaluations []FeatureToggleEvaluation
	// ContentHash is the Base64-encoded hash from the ContentHash response header,
	// used for efficient change detection
	ContentHash []byte
}

// FeatureToggleEvaluation represents a single feature toggle with its
// configuration and targeting rules.
type FeatureToggleEvaluation struct {
	// Name is the human-readable name of the feature toggle
	Name string `json:"name"`
	// Slug is the unique identifier used for feature flag evaluation
	Slug string `json:"slug"`
	// IsEnabled indicates whether the feature toggle is enabled
	IsEnabled bool `json:"isEnabled"`
	// Segments contains the targeting rules for this feature toggle.
	// Each segment is a map of key-value pairs that must match the evaluation context.
	Segments []map[string]string `json:"segments"`
}
