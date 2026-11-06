package store

import (
	"github.com/chhongzh/scribekit/internal/apperr"
)

func notFound(what string, id int) *apperr.Error {
	return apperr.New(apperr.NotFound, "%s %d 不存在", what, id)
}

func invalid(format string, args ...any) *apperr.Error {
	return apperr.New(apperr.BadRequest, format, args...)
}

func conflict(format string, args ...any) *apperr.Error {
	return apperr.New(apperr.Conflict, format, args...)
}
