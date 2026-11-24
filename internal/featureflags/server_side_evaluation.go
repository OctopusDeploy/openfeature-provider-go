package featureflags

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/open-feature/go-sdk/openfeature"
)

// ServerSideEvaluation is one flag from the v4 evaluations endpoint, either
// resolved by the server (Value and Reason) or deferred to the client
// (EvaluationKey and Rules). Pointers and a nil Rules slice let a missing
// field be told apart from false, "" or [].
type ServerSideEvaluation struct {
	Slug          string            `json:"slug"`
	Value         *bool             `json:"value"`
	Reason        *string           `json:"reason"`
	EvaluationKey *string           `json:"evaluationKey"`
	Rules         []*ClientSideRule `json:"rules"`
}

// ErrNullEvaluations is returned by [ParseEvaluations] for a JSON null body.
var ErrNullEvaluations = errors.New("the evaluations response was null")

// ParseEvaluations decodes the body of a v4 evaluations response. Null
// entries and entries without a slug are dropped, since no lookup can match
// them. A wrong-typed field fails the whole response.
func ParseEvaluations(body []byte) ([]ServerSideEvaluation, error) {
	var entries []*ServerSideEvaluation
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("decoding the evaluations response: %w", err)
	}

	if entries == nil {
		return nil, ErrNullEvaluations
	}

	evaluations := make([]ServerSideEvaluation, 0, len(entries))
	for _, entry := range entries {
		if entry == nil || entry.Slug == "" {
			continue
		}
		evaluations = append(evaluations, *entry)
	}

	return evaluations, nil
}

// Evaluate resolves the flag, returning its value and reason. A flag deferred
// to the client is enabled when any of its rules matches. A malformed flag
// returns a PARSE_ERROR [openfeature.ResolutionError].
func (e ServerSideEvaluation) Evaluate(flatCtx openfeature.FlattenedContext) (bool, string, error) {
	if e.Value != nil {
		if e.Reason == nil {
			return false, "", parseError("The flag has a value but has no reason.")
		}

		if e.EvaluationKey != nil || e.Rules != nil {
			return false, "", parseError("The flag has both a server-resolved value and client-side rules.")
		}

		return *e.Value, *e.Reason, nil
	}

	if e.Rules == nil {
		return false, "", parseError("The flag has neither a value nor rules.")
	}

	if e.EvaluationKey == nil {
		return false, "", parseError("The flag defers to the client but has no evaluation key.")
	}

	if len(e.Rules) == 0 {
		return false, "", parseError("The flag defers to the client with no rules.")
	}

	ruleCtx := ruleContext{evaluationKey: *e.EvaluationKey, flatCtx: flatCtx}

	for _, rule := range e.Rules {
		if rule == nil {
			return false, "", parseError("The flag has a missing rule.")
		}

		matched, err := rule.matches(ruleCtx)
		if err != nil {
			return false, "", err
		}

		if matched {
			return true, matchedRuleReason(*rule.Name), nil
		}
	}

	return false, didNotMatchAnyRulesReason, nil
}

func parseError(message string) error {
	return openfeature.NewParseErrorResolutionError(message)
}
