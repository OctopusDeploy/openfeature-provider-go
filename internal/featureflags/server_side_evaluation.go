package featureflags

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/open-feature/go-sdk/openfeature"
)

// ServerSideEvaluation is one flag from the v4 evaluations endpoint. Value and
// Reason are pointers so a missing field can be told apart from false or "".
type ServerSideEvaluation struct {
	Slug   string  `json:"slug"`
	Value  *bool   `json:"value"`
	Reason *string `json:"reason"`
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

// Evaluate resolves the flag, returning its value and reason. A malformed
// flag returns a PARSE_ERROR [openfeature.ResolutionError].
func (e ServerSideEvaluation) Evaluate(_ openfeature.FlattenedContext) (bool, string, error) {
	if e.Value == nil {
		return false, "", openfeature.NewParseErrorResolutionError("The flag has neither a value nor rules.")
	}

	if e.Reason == nil {
		return false, "", openfeature.NewParseErrorResolutionError("The flag has a value but has no reason.")
	}

	return *e.Value, *e.Reason, nil
}
