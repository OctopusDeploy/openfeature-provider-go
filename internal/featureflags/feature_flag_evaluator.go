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
	evaluationsBySlug map[string]ServerSideEvaluation
	contentHash       []byte
	logger            *slog.Logger
	warnedSlugs       sync.Map
}

// NewFeatureFlagEvaluator returns an evaluator for response. The logger must
// not be nil.
func NewFeatureFlagEvaluator(response EvaluationResponse, logger *slog.Logger) *FeatureFlagEvaluator {
	evaluationsBySlug := make(map[string]ServerSideEvaluation, len(response.Evaluations))
	for _, evaluation := range response.Evaluations {
		key := slugKey(evaluation.Slug)
		if _, exists := evaluationsBySlug[key]; !exists {
			evaluationsBySlug[key] = evaluation
		}
	}

	return &FeatureFlagEvaluator{
		evaluationsBySlug: evaluationsBySlug,
		contentHash:       response.ContentHash,
		logger:            logger,
	}
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
	return e.contentHash
}

// FindEvaluationBySlug returns the first flag whose slug matches,
// ignoring case.
func (e *FeatureFlagEvaluator) FindEvaluationBySlug(slug string) (ServerSideEvaluation, bool) {
	evaluation, found := e.evaluationsBySlug[slugKey(slug)]
	return evaluation, found
}

// slugKey is the one place slugs are made case-insensitive, so the index and
// the warn-once tracking can't disagree about which slugs are the same.
func slugKey(slug string) string {
	return strings.ToLower(slug)
}

// Evaluate resolves the flag with the given slug. On any error it returns
// defaultValue alongside the error. A missing slug logs a warning the first
// time it is asked for, ignoring case.
func (e *FeatureFlagEvaluator) Evaluate(slug string, defaultValue bool, flatCtx openfeature.FlattenedContext) openfeature.BoolResolutionDetail {
	evaluation, found := e.findEvaluationOrWarn(slug)
	if !found {
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

// RejectNonBooleanEvaluation resolves a request for a non-boolean flag value,
// which Octopus doesn't support. It returns TYPE_MISMATCH for an existing
// flag, and FLAG_NOT_FOUND for a missing one, warning as
// [FeatureFlagEvaluator.Evaluate] does.
func (e *FeatureFlagEvaluator) RejectNonBooleanEvaluation(slug string) openfeature.ProviderResolutionDetail {
	err := openfeature.NewTypeMismatchResolutionError("Octopus only supports boolean flags.")
	if _, found := e.findEvaluationOrWarn(slug); !found {
		err = openfeature.NewFlagNotFoundResolutionError(flagNotFoundMessage)
	}

	return openfeature.ProviderResolutionDetail{ResolutionError: err, Reason: openfeature.ErrorReason}
}

func (e *FeatureFlagEvaluator) findEvaluationOrWarn(slug string) (ServerSideEvaluation, bool) {
	evaluation, found := e.FindEvaluationBySlug(slug)
	if !found {
		if _, warned := e.warnedSlugs.LoadOrStore(slugKey(slug), struct{}{}); !warned {
			e.logger.Warn("The slug did not match any of your Octopus Feature Flags. Please double check your slug and try again.", slog.String("slug", slug))
		}
	}

	return evaluation, found
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
