package featureflags

import "encoding/json"

// These mirror the type values in the evaluation response, so they must not
// drift from the server.
const (
	contextAttributeIsOneOfType    = "context-attribute-is-one-of"
	contextAttributeIsNotOneOfType = "context-attribute-is-not-one-of"
	percentageByContextType        = "percentage-by-context"
)

type conditionMatcher interface {
	// matches reports whether the condition is met. A condition that arrived
	// in a shape its type can't evaluate returns a PARSE_ERROR.
	matches(ctx ruleContext) (bool, error)
}

// ClientSideCondition is one condition of a [ClientSideRule], decoded by its
// type.
type ClientSideCondition struct {
	matcher conditionMatcher
}

func (c ClientSideCondition) matches(ctx ruleContext) (bool, error) {
	return c.matcher.matches(ctx)
}

// UnmarshalJSON reads type first and decodes only the fields that type
// needs. An unrecognised type keeps none of its fields, so a condition
// introduced by a newer server can't fail an older client's response.
func (c *ClientSideCondition) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}

	// A non-string type is treated as absent. The lookup is case-sensitive,
	// as it is in .NET.
	var conditionType *string
	if rawType, found := fields["type"]; found {
		if err := json.Unmarshal(rawType, &conditionType); err != nil {
			conditionType = nil
		}
	}

	if conditionType == nil {
		c.matcher = unknownCondition{}
		return nil
	}

	var err error
	switch *conditionType {
	case contextAttributeIsOneOfType:
		c.matcher, err = decodeCondition[contextAttributeIsOneOfCondition](data)
	case contextAttributeIsNotOneOfType:
		c.matcher, err = decodeCondition[contextAttributeIsNotOneOfCondition](data)
	case percentageByContextType:
		c.matcher, err = decodeCondition[percentageByContextCondition](data)
	default:
		c.matcher = unknownCondition{conditionType: conditionType}
	}

	return err
}

func decodeCondition[T conditionMatcher](data []byte) (conditionMatcher, error) {
	var condition T
	err := json.Unmarshal(data, &condition)
	return condition, err
}

// unknownCondition is a condition whose type this provider doesn't
// recognise. It never matches, so a newer server's capability is treated as
// not met rather than failing the flag.
type unknownCondition struct {
	conditionType *string
}

func (c unknownCondition) matches(ruleContext) (bool, error) {
	// No server version sends a condition without a type, so unlike an
	// unrecognised type this is a response that could not have been sent.
	if c.conditionType == nil {
		return false, parseError("A condition is missing a type.")
	}

	return false, nil
}
