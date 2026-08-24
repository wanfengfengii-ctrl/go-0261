package store

import (
	"errors"
	"fmt"
	"sort"
)

// Stable error codes surfaced by the JSON API. They are part of the public
// contract (failure boundary #2) and are asserted by tests.
const (
	CodeNotFound             = "NOT_FOUND"
	CodeIdempotencyConflict  = "IDEMPOTENCY_CONFLICT"
	CodeStaleRevision        = "STALE_REVISION"
	CodeSealMismatch         = "SEAL_MISMATCH"
	CodeOccupied             = "OCCUPIED"
	CodeInvalidState         = "INVALID_STATE"
	CodeTerminalState        = "TERMINAL_STATE"
	CodeGenerationMismatch   = "GENERATION_MISMATCH"
	CodeInvalidReading       = "INVALID_READING"
	CodeDuplicateTime        = "DUPLICATE_TIME"
	CodeMissingTime          = "MISSING_TIME"
	CodeCoverageIncomplete   = "COVERAGE_INCOMPLETE"
	CodeReviewOverlap        = "REVIEW_OVERLAP"
	CodeReviewDuplicate      = "REVIEW_DUPLICATE"
	CodePersonNotQualified   = "PERSON_NOT_QUALIFIED"
	CodeBlindCodeRevealed    = "BLIND_CODE_REVEALED"
	CodeBlindCodeDuplicate   = "BLIND_CODE_DUPLICATE"
	CodeRecheckAlreadyExists = "RECHECK_ALREADY_EXISTS"
	CodeArithmeticError      = "ARITHMETIC_ERROR"
	CodeFinalizeConflict     = "FINALIZE_CONFLICT"
	CodeAnomalyPresent       = "ANOMALY_PRESENT"
	CodeValidationFailed     = "VALIDATION_FAILED"
	CodeUnknownBlindCode     = "UNKNOWN_BLIND_CODE"
)

// AppError is the structured rejection returned by every write endpoint. It
// carries a stable code and a deterministically sorted reason list.
type AppError struct {
	Code    string   `json:"error_code"`
	Reasons []string `json:"reasons"`
}

// Error implements the error interface.
func (e *AppError) Error() string {
	return fmt.Sprintf("%s: %v", e.Code, e.Reasons)
}

// NewAppError builds an AppError with a sorted, de-duplicated reason list so
// that identical logical failures produce identical responses (failure
// boundary #2).
func NewAppError(code string, reasons ...string) *AppError {
	reasons = append([]string(nil), reasons...)
	sort.Strings(reasons)
	reasons = dedupe(reasons)
	return &AppError{Code: code, Reasons: reasons}
}

// IsAppError reports whether err is (or wraps) an AppError with the given code.
func IsAppError(err error, code string) bool {
	var ae *AppError
	if errors.As(err, &ae) {
		return ae.Code == code
	}
	return false
}

// AppErrorCode returns the code of an AppError, or "" when err is not one.
func AppErrorCode(err error) string {
	var ae *AppError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return ""
}

func dedupe(in []string) []string {
	if len(in) == 0 {
		return in
	}
	out := in[:1]
	for _, s := range in[1:] {
		if s != out[len(out)-1] {
			out = append(out, s)
		}
	}
	return out
}
