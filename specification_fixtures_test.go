package octopus

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/open-feature/go-sdk/openfeature"
)

// Read at runtime rather than with go:embed, because module zips leave out
// submodules.
const specificationFixturesGlob = "specification/Fixtures/*.json"

type specificationFixture struct {
	Response json.RawMessage     `json:"response"`
	Cases    []specificationCase `json:"cases"`
}

type specificationCase struct {
	Description   string                     `json:"description"`
	Configuration specificationConfiguration `json:"configuration"`
	Expected      specificationExpectation   `json:"expected"`
}

type specificationConfiguration struct {
	Slug         string `json:"slug"`
	DefaultValue bool   `json:"defaultValue"`
	// A null attribute is passed on as a nil value rather than left out.
	Context map[string]any `json:"context"`
}

type specificationExpectation struct {
	Value bool `json:"value"`
	// Fixtures leave out the reason on errors, so nil means it isn't checked.
	Reason *openfeature.Reason `json:"reason"`
	// Empty when the evaluation returns no error.
	ErrorCode openfeature.ErrorCode `json:"errorCode"`
}

type evaluateSpecificationCase func(t *testing.T, response json.RawMessage, configuration specificationConfiguration) openfeature.BooleanEvaluationDetails

func TestSpecificationFixtures(t *testing.T) {
	runSpecificationFixtures(t, evaluateThroughProvider)
}

// evaluateThroughProvider evaluates through the SDK, so the tests also cover
// how it handles errors and flattens the context.
func evaluateThroughProvider(t *testing.T, response json.RawMessage, configuration specificationConfiguration) openfeature.BooleanEvaluationDetails {
	t.Helper()

	server := newFakeFeatureFlagsServer(t)
	server.setEvaluations(fakeResponse{body: string(response), contentHash: []byte("fixture-hash")})
	client := startProvider(t, server.configuration(t))

	// The specification's targetingKey is the OpenFeature targeting key, not
	// a custom attribute.
	targetingKey := ""
	attributes := make(map[string]any, len(configuration.Context))
	for key, value := range configuration.Context {
		if key == openfeature.TargetingKey {
			targetingKey, _ = value.(string)
			continue
		}
		attributes[key] = value
	}

	// The details carry the error code, which is what the fixtures check.
	details, _ := client.BooleanValueDetails(
		context.Background(),
		configuration.Slug,
		configuration.DefaultValue,
		openfeature.NewEvaluationContext(targetingKey, attributes),
	)

	return details
}

func runSpecificationFixtures(t *testing.T, evaluate evaluateSpecificationCase) {
	paths, err := filepath.Glob(specificationFixturesGlob)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatalf("no specification fixtures match %s. Clone with --recurse-submodules, or run git submodule update --init", specificationFixturesGlob)
	}

	for _, path := range paths {
		fixture := readSpecificationFixture(t, path)

		for _, testCase := range fixture.Cases {
			t.Run(filepath.Base(path)+" - "+testCase.Description, func(t *testing.T) {
				details := evaluate(t, fixture.Response, testCase.Configuration)

				expected := testCase.Expected
				if details.Value != expected.Value {
					t.Errorf("Value = %v, want %v", details.Value, expected.Value)
				}
				if details.ErrorCode != expected.ErrorCode {
					t.Errorf("ErrorCode = %q, want %q (error message %q)", details.ErrorCode, expected.ErrorCode, details.ErrorMessage)
				}
				if expected.Reason != nil && details.Reason != *expected.Reason {
					t.Errorf("Reason = %q, want %q", details.Reason, *expected.Reason)
				}
			})
		}
	}
}

func readSpecificationFixture(t *testing.T, path string) specificationFixture {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var fixture specificationFixture
	if err := json.Unmarshal(content, &fixture); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	if len(fixture.Cases) == 0 {
		t.Fatalf("%s has no cases", path)
	}

	return fixture
}
