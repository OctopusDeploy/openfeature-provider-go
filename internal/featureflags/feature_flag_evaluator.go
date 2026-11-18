package featureflags

import (
	"errors"
	"log/slog"
	"strings"
	"sync"

	"github.com/open-feature/go-sdk/openfeature"
)

const flagNotFoundMessage = "The slug provided did not match any of your Octopus Feature Flags. Please double check your slug and try again."

// FeatureFlagEvaluator resolves flags from one evaluation response. It is
// safe for concurrent use.
type FeatureFlagEvaluator struct {
	response    EvaluationResponse
	logger      *slog.Logger
	warnedSlugs sync.Map
}

// NewFeatureFlagEvaluator returns an evaluator for response. The logger must
// not be nil.
func NewFeatureFlagEvaluator(response EvaluationResponse, logger *slog.Logger) *FeatureFlagEvaluator {
	return &FeatureFlagEvaluator{response: response, logger: logger}
}

// EmptyFeatureFlagEvaluator returns an evaluator with no flags, for use
// before a response has been fetched. Every evaluation returns
// FLAG_NOT_FOUND.
func EmptyFeatureFlagEvaluator(logger *slog.Logger) *FeatureFlagEvaluator {
	return NewFeatureFlagEvaluator(EvaluationResponse{}, logger)
}

// ContentHash returns the content hash of the response the evaluator was
// built from.
func (e *FeatureFlagEvaluator) ContentHash() []byte {
	return e.response.ContentHash
}

// FindEvaluationBySlug returns the first flag whose slug matches,
// ignoring case.
func (e *FeatureFlagEvaluator) FindEvaluationBySlug(slug string) (ServerSideEvaluation, bool) {
	key := slugKey(slug)
	for _, evaluation := range e.response.Evaluations {
		if slugKey(evaluation.Slug) == key {
			return evaluation, true
		}
	}

	return ServerSideEvaluation{}, false
}

// slugKey is the one place slugs are made case-insensitive, so the lookup and
// the warn-once tracking can't disagree about which slugs are the same.
func slugKey(slug string) string {
	return strings.ToLower(slug)
}

// Evaluate resolves the flag with the given slug. On any error it returns
// defaultValue alongside the error. A missing slug logs a warning the first
// time it is asked for, ignoring case.
func (e *FeatureFlagEvaluator) Evaluate(slug string, defaultValue bool, flatCtx openfeature.FlattenedContext) openfeature.BoolResolutionDetail {
	evaluation, found := e.FindEvaluationBySlug(slug)
	if !found {
		if _, warned := e.warnedSlugs.LoadOrStore(slugKey(slug), struct{}{}); !warned {
			e.logger.Warn("The slug did not match any of your Octopus Feature Flags. Please double check your slug and try again.", slog.String("slug", slug))
		}

		return failed(defaultValue, openfeature.NewFlagNotFoundResolutionError(flagNotFoundMessage))
	}

	value, reason, err := evaluation.Evaluate(flatCtx)
	if err != nil {
		var resolutionErr openfeature.ResolutionError
		if !errors.As(err, &resolutionErr) {
			resolutionErr = openfeature.NewGeneralResolutionError(err.Error())
		}

		return failed(defaultValue, resolutionErr)
	}

	return openfeature.BoolResolutionDetail{
		Value:                    value,
		ProviderResolutionDetail: openfeature.ProviderResolutionDetail{Reason: openfeature.Reason(reason)},
	}
}

func failed(defaultValue bool, err openfeature.ResolutionError) openfeature.BoolResolutionDetail {
	return openfeature.BoolResolutionDetail{
		Value: defaultValue,
		ProviderResolutionDetail: openfeature.ProviderResolutionDetail{
			ResolutionError: err,
			Reason:          openfeature.ErrorReason,
		},
	}
}
