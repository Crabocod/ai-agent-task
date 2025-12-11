package apperr

import "fmt"

const (
	MetaReason   = "reason"
	MetaStage    = "stage"
	MetaField    = "field"
	MetaUserID   = "user_id"
	MetaTaskID   = "task_id"
	MetaAction   = "action"
	MetaSelector = "selector"
	MetaURL      = "url"

	StagePreparation  = "preparation"
	StageBrowser      = "browser"
	StageAI           = "ai"
	StageExecution    = "execution"
	StageScreenshot   = "screenshot"
	StagePageState    = "page_state"
	StageNavigation   = "navigation"
	StageInteraction  = "interaction"

	CodeInternal          = "internal"
	CodeInvalidArgument   = "invalid_argument"
	CodeNotFound          = "not_found"
	CodeUnavailable       = "unavailable"
	CodeTimeout           = "timeout"
	CodeMaxIterations     = "max_iterations"
	CodeDuplicateAction   = "duplicate_action"
	CodeCancelledByUser   = "cancelled_by_user"
	CodeBrowserNotReady   = "browser_not_ready"
	CodeActionFailed      = "action_failed"
	CodeAIError           = "ai_error"
)

type Error struct {
	Op       string
	Code     string
	Err      error
	Metadata map[string]any
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Op, e.Err)
	}

	return e.Op
}

func (e *Error) Unwrap() error {
	return e.Err
}

func Wrap(op, code string, err error, metadata map[string]any) error {
	if metadata == nil {
		metadata = make(map[string]any)
	}

	return &Error{
		Op:       op,
		Code:     code,
		Err:      err,
		Metadata: metadata,
	}
}

func WrapWithReason(op, code string, err error, reason string) error {
	return Wrap(op, code, err, map[string]any{
		MetaReason: reason,
	})
}

func WrapErrorWithReason(op, code, reason string) error {
	return Wrap(op, code, fmt.Errorf(reason), map[string]any{
		MetaReason: reason,
	})
}

func InvalidReqError(op, field string, err error) error {
	return Wrap(op, CodeInvalidArgument, err, map[string]any{
		MetaField:  field,
		MetaReason: "invalid_request",
	})
}

func NotFoundError(op string, err error) error {
	return Wrap(op, CodeNotFound, err, map[string]any{
		MetaReason: "not_found",
	})
}
