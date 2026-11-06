// Package apperr 定义前后端约定的错误码，REST 与 WebSocket 共用。
package apperr

import (
	"context"
	"errors"
	"fmt"
)

const (
	ModelNotConfigured = "model_not_configured"
	NeedsUserInput     = "needs_user_input"
	UserOwnedParagraph = "user_owned_paragraph"
	Cancelled          = "cancelled"
	BadRequest         = "bad_request"
	NotFound           = "not_found"
	Conflict           = "conflict"
	Internal           = "internal"
)

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Message }

func New(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// From 把任意错误归类；未识别的错误视为 internal，调用方负责记录日志。
func From(err error) *Error {
	var ae *Error
	switch {
	case errors.As(err, &ae):
		return ae
	case errors.Is(err, context.Canceled):
		return &Error{Code: Cancelled, Message: "请求已取消"}
	default:
		return &Error{Code: Internal, Message: "服务内部错误"}
	}
}
