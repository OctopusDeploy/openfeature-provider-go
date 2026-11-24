package featureflags

import (
	"bytes"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/open-feature/go-sdk/openfeature"
)

func newTestEvaluator(t *testing.T, body string, logOutput io.Writer) *FeatureFlagEvaluator {
	t.Helper()

	evaluations, err := ParseEvaluations([]byte(body))
	if err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(logOutput, nil))

	return NewFeatureFlagEvaluator(EvaluationResponse{Evaluations: evaluations}, logger)
}

func TestEvaluate(t *testing.T) {
	const body = `[
		{"slug": "enabled-flag", "value": true, "reason": "The flag is enabled for this environment."},
		{"slug": "disabled-flag", "value": false, "reason": "The flag is disabled for this environment."},
		{"slug": "duplicate-flag", "value": true, "reason": "First match."},
		{"slug": "DUPLICATE-FLAG", "value": false, "reason": "Second match."},
		{"slug": "flag-with-no-value", "reason": "No value."},
		{"slug": "flag-with-no-reason", "value": true}
	]`

	tests := []struct {
		name          string
		slug          string
		defaultValue  bool
		wantValue     bool
		wantReason    openfeature.Reason
		wantErrorCode openfeature.ErrorCode
	}{
		{
			name:       "returns the server's value and reason",
			slug:       "enabled-flag",
			wantValue:  true,
			wantReason: "The flag is enabled for this environment.",
		},
		{
			name:         "returns a false value from the server",
			slug:         "disabled-flag",
			defaultValue: true,
			wantValue:    false,
			wantReason:   "The flag is disabled for this environment.",
		},
		{
			name:       "matches the slug ignoring case",
			slug:       "ENABLED-FLAG",
			wantValue:  true,
			wantReason: "The flag is enabled for this environment.",
		},
		{
			name:       "takes the first match over a later exact match",
			slug:       "DUPLICATE-FLAG",
			wantValue:  true,
			wantReason: "First match.",
		},
		{
			name:          "returns the default when the slug is missing",
			slug:          "missing-flag",
			defaultValue:  true,
			wantValue:     true,
			wantReason:    openfeature.ErrorReason,
			wantErrorCode: openfeature.FlagNotFoundCode,
		},
		{
			name:          "returns a parse error for a flag with no value",
			slug:          "flag-with-no-value",
			defaultValue:  true,
			wantValue:     true,
			wantReason:    openfeature.ErrorReason,
			wantErrorCode: openfeature.ParseErrorCode,
		},
		{
			name:          "returns a parse error for a flag with a value but no reason",
			slug:          "flag-with-no-reason",
			wantValue:     false,
			wantReason:    openfeature.ErrorReason,
			wantErrorCode: openfeature.ParseErrorCode,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evaluator := newTestEvaluator(t, body, io.Discard)

			result := evaluator.Evaluate(tt.slug, tt.defaultValue, openfeature.FlattenedContext{})
			detail := result.ResolutionDetail()

			if result.Value != tt.wantValue {
				t.Errorf("Value = %v, want %v", result.Value, tt.wantValue)
			}
			if detail.Reason != tt.wantReason {
				t.Errorf("Reason = %q, want %q", detail.Reason, tt.wantReason)
			}
			if detail.ErrorCode != tt.wantErrorCode {
				t.Errorf("ErrorCode = %q, want %q", detail.ErrorCode, tt.wantErrorCode)
			}
		})
	}
}

// The shared specification fixtures cover client-side rules. These are the
// cases they can't express: wrong-typed fields, the case of the type key and
// non-string context values.
func TestEvaluateClientSideRulesBeyondTheFixtures(t *testing.T) {
	const body = `[
		{"slug": "unknown-type-with-wrong-typed-fields", "evaluationKey": "key", "rules": [
			{"name": "Future", "conditions": [{"type": "some-future-condition", "key": 1, "values": {"a": true}, "percentage": "50"}]}
		]},
		{"slug": "non-string-type", "evaluationKey": "key", "rules": [
			{"name": "Typed", "conditions": [{"type": 1}]}
		]},
		{"slug": "capitalised-type-key", "evaluationKey": "key", "rules": [
			{"name": "Everyone", "conditions": [{"Type": "percentage-by-context", "percentage": 100}]}
		]},
		{"slug": "partial-rollout", "evaluationKey": "key", "rules": [
			{"name": "Rollout", "conditions": [{"type": "percentage-by-context", "percentage": 99}]}
		]},
		{"slug": "region-is-one-of", "evaluationKey": "key", "rules": [
			{"name": "Region", "conditions": [{"type": "context-attribute-is-one-of", "key": "region", "values": ["1", "true"]}]}
		]},
		{"slug": "region-is-not-one-of", "evaluationKey": "key", "rules": [
			{"name": "Region", "conditions": [{"type": "context-attribute-is-not-one-of", "key": "region", "values": ["1", "true"]}]}
		]}
	]`

	tests := []struct {
		name          string
		slug          string
		context       openfeature.FlattenedContext
		wantValue     bool
		wantReason    openfeature.Reason
		wantErrorCode openfeature.ErrorCode
	}{
		{
			name:       "an unrecognised condition type ignores its wrong-typed fields and doesn't match",
			slug:       "unknown-type-with-wrong-typed-fields",
			wantReason: "Did not match any rules.",
		},
		{
			name:          "a non-string condition type is a parse error",
			slug:          "non-string-type",
			wantReason:    openfeature.ErrorReason,
			wantErrorCode: openfeature.ParseErrorCode,
		},
		{
			name:          "the type key is case-sensitive, so a capitalised one is a missing type",
			slug:          "capitalised-type-key",
			wantReason:    openfeature.ErrorReason,
			wantErrorCode: openfeature.ParseErrorCode,
		},
		{
			name:       "a non-string targeting key is missing, so only a full rollout matches",
			slug:       "partial-rollout",
			context:    openfeature.FlattenedContext{openfeature.TargetingKey: 1},
			wantReason: "Did not match any rules.",
		},
		{
			name:       "an int attribute is absent to is-one-of",
			slug:       "region-is-one-of",
			context:    openfeature.FlattenedContext{"region": 1},
			wantReason: "Did not match any rules.",
		},
		{
			name:       "a bool attribute is absent to is-one-of",
			slug:       "region-is-one-of",
			context:    openfeature.FlattenedContext{"region": true},
			wantReason: "Did not match any rules.",
		},
		{
			name:       "a nil attribute is absent to is-one-of",
			slug:       "region-is-one-of",
			context:    openfeature.FlattenedContext{"region": nil},
			wantReason: "Did not match any rules.",
		},
		{
			name:       "an int attribute is absent to is-not-one-of",
			slug:       "region-is-not-one-of",
			context:    openfeature.FlattenedContext{"region": 1},
			wantValue:  true,
			wantReason: "Matched rule 'Region'.",
		},
		{
			name:       "a bool attribute is absent to is-not-one-of",
			slug:       "region-is-not-one-of",
			context:    openfeature.FlattenedContext{"region": true},
			wantValue:  true,
			wantReason: "Matched rule 'Region'.",
		},
		{
			name:       "a nil attribute is absent to is-not-one-of",
			slug:       "region-is-not-one-of",
			context:    openfeature.FlattenedContext{"region": nil},
			wantValue:  true,
			wantReason: "Matched rule 'Region'.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evaluator := newTestEvaluator(t, body, io.Discard)

			result := evaluator.Evaluate(tt.slug, false, tt.context)
			detail := result.ResolutionDetail()

			if result.Value != tt.wantValue {
				t.Errorf("Value = %v, want %v", result.Value, tt.wantValue)
			}
			if detail.Reason != tt.wantReason {
				t.Errorf("Reason = %q, want %q", detail.Reason, tt.wantReason)
			}
			if detail.ErrorCode != tt.wantErrorCode {
				t.Errorf("ErrorCode = %q, want %q", detail.ErrorCode, tt.wantErrorCode)
			}
		})
	}
}

func TestEvaluateWarnsOncePerMissingSlugIgnoringCase(t *testing.T) {
	var logOutput bytes.Buffer
	evaluator := newTestEvaluator(t, `[]`, &logOutput)

	for _, slug := range []string{"missing-flag", "MISSING-FLAG", "missing-flag", "other-flag"} {
		evaluator.Evaluate(slug, false, openfeature.FlattenedContext{})
	}

	if warnings := strings.Count(logOutput.String(), "level=WARN"); warnings != 2 {
		t.Errorf("logged %d warnings, want 2:\n%s", warnings, logOutput.String())
	}
}

func TestEmptyEvaluatorReturnsFlagNotFound(t *testing.T) {
	evaluator := EmptyFeatureFlagEvaluator(slog.New(slog.DiscardHandler))

	result := evaluator.Evaluate("any-flag", true, openfeature.FlattenedContext{})

	if !result.Value {
		t.Error("Value = false, want the default true")
	}
	if code := result.ResolutionDetail().ErrorCode; code != openfeature.FlagNotFoundCode {
		t.Errorf("ErrorCode = %q, want %q", code, openfeature.FlagNotFoundCode)
	}
}
