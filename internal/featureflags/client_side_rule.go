package featureflags

import (
	"fmt"

	"github.com/open-feature/go-sdk/openfeature"
)

// Both reasons match the ones the feature flags service gives server-side, so
// a flag reads the same whichever side resolved it.
const didNotMatchAnyRulesReason = "Did not match any rules."

func matchedRuleReason(ruleName string) string {
	return fmt.Sprintf("Matched rule '%s'.", ruleName)
}

// ruleContext is what a flag's rules and conditions are evaluated against.
type ruleContext struct {
	// evaluationKey is what percentage-by-context buckets against.
	evaluationKey string
	flatCtx       openfeature.FlattenedContext
}

// ClientSideRule is a named rule the server left for the client to evaluate.
// It matches when every one of its conditions matches.
type ClientSideRule struct {
	Name       *string                `json:"name"`
	Conditions []*ClientSideCondition `json:"conditions"`
}

// matches evaluates the conditions in order, stopping at the first that
// doesn't match, so a malformed condition after it is never reached.
func (r ClientSideRule) matches(ruleCtx ruleContext) (bool, error) {
	// The server only defers a named rule with at least one condition, so
	// anything else means the response is malformed.
	if r.Name == nil {
		return false, parseError("A rule has no name.")
	}

	if len(r.Conditions) == 0 {
		return false, parseError(fmt.Sprintf("Rule '%s' has no conditions.", *r.Name))
	}

	for _, condition := range r.Conditions {
		if condition == nil {
			return false, parseError(fmt.Sprintf("Rule '%s' has a missing condition.", *r.Name))
		}

		matched, err := condition.matches(ruleCtx)
		if err != nil || !matched {
			return false, err
		}
	}

	return true, nil
}
