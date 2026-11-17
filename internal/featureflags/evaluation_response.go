package featureflags

// EvaluationResponse is one v4 evaluations response: its flags, and the
// content hash the check endpoint compares against to detect changes.
type EvaluationResponse struct {
	Evaluations []ServerSideEvaluation
	ContentHash []byte
}
