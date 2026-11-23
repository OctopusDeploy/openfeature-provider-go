package featureflags

import "testing"

func TestParseEvaluationsSkipsNullAndSluglessEntries(t *testing.T) {
	evaluations, err := ParseEvaluations([]byte(`[
		null,
		{"value": true, "reason": "No slug."},
		{"slug": null, "value": true, "reason": "Null slug."},
		{"slug": "feature-a", "value": true, "reason": "Enabled."}
	]`))
	if err != nil {
		t.Fatal(err)
	}

	if len(evaluations) != 1 || evaluations[0].Slug != "feature-a" {
		t.Errorf("evaluations = %+v, want only feature-a", evaluations)
	}
}

func TestParseEvaluationsRejectsANullBody(t *testing.T) {
	if _, err := ParseEvaluations([]byte(`null`)); err == nil {
		t.Error("expected an error for a null body")
	}
}

func TestParseEvaluationsRejectsTheWholeBodyForAWrongTypedField(t *testing.T) {
	_, err := ParseEvaluations([]byte(`[
		{"slug": "feature-a", "value": "yes", "reason": "Wrong type."},
		{"slug": "feature-b", "value": true, "reason": "Enabled."}
	]`))
	if err == nil {
		t.Error("expected an error for a wrong-typed field")
	}
}

func TestParseEvaluationsRejectsTheWholeBodyForAWrongTypedRuleField(t *testing.T) {
	tests := []struct {
		name  string
		rules string
	}{
		{
			name:  "rules is an object",
			rules: `{"name": "Rule", "conditions": []}`,
		},
		{
			name:  "percentage is a string",
			rules: `[{"name": "Rule", "conditions": [{"type": "percentage-by-context", "percentage": "50"}]}]`,
		},
		{
			name:  "key is a number",
			rules: `[{"name": "Rule", "conditions": [{"type": "context-attribute-is-one-of", "key": 1, "values": ["a"]}]}]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := `[
				{"slug": "feature-a", "evaluationKey": "key", "rules": ` + tt.rules + `},
				{"slug": "feature-b", "value": true, "reason": "Enabled."}
			]`

			if _, err := ParseEvaluations([]byte(body)); err == nil {
				t.Error("expected an error for a wrong-typed field")
			}
		})
	}
}

func TestParseEvaluationsAcceptsAnEmptyArray(t *testing.T) {
	evaluations, err := ParseEvaluations([]byte(`[]`))
	if err != nil {
		t.Fatal(err)
	}

	if len(evaluations) != 0 {
		t.Errorf("evaluations = %+v, want none", evaluations)
	}
}
