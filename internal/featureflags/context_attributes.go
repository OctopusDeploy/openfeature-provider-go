package featureflags

import (
	"slices"
	"strings"
)

// contextAttributeIsOneOfCondition matches when the context attribute Key is
// one of Values. A missing attribute doesn't match.
type contextAttributeIsOneOfCondition struct {
	Key    *string   `json:"key"`
	Values []*string `json:"values"`
}

func (c contextAttributeIsOneOfCondition) matches(ctx ruleContext) (bool, error) {
	return isOneOf(ctx, c.Key, c.Values)
}

// contextAttributeIsNotOneOfCondition is the exact negation of
// contextAttributeIsOneOfCondition, so a missing attribute matches.
type contextAttributeIsNotOneOfCondition contextAttributeIsOneOfCondition

func (c contextAttributeIsNotOneOfCondition) matches(ctx ruleContext) (bool, error) {
	isOne, err := isOneOf(ctx, c.Key, c.Values)
	if err != nil {
		return false, err
	}

	return !isOne, nil
}

// isOneOf reports whether the context holds an attribute named key with one
// of values. Keys and values compare ignoring case, and a non-string value
// counts as absent. Every entry whose key matches is checked, since a context
// can hold several casings of one key and map order is random.
func isOneOf(ctx ruleContext, key *string, values []*string) (bool, error) {
	if key == nil {
		return false, parseError("A condition is missing a key.")
	}

	if len(values) == 0 {
		return false, parseError("A condition is missing values.")
	}

	if slices.Contains(values, nil) {
		return false, parseError("A condition is missing a value.")
	}

	for attributeKey, attributeValue := range ctx.flatCtx {
		attribute, isString := attributeValue.(string)
		if !isString || !strings.EqualFold(attributeKey, *key) {
			continue
		}

		if slices.ContainsFunc(values, func(value *string) bool { return strings.EqualFold(*value, attribute) }) {
			return true, nil
		}
	}

	return false, nil
}
