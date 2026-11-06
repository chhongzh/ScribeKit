package analyzer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/chhongzh/scribekit/internal/llm"
)

func TestAskExitsWhenQueueDeadlineExpires(t *testing.T) {
	service := &Service{sem: make(chan struct{}, 1)}
	service.sem <- struct{}{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := ask(ctx, service, "test", llm.Handle{}, nil, func(string) (string, error) {
		t.Fatal("model should not run while slot is occupied")
		return "", nil
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline, got %v", err)
	}
}
