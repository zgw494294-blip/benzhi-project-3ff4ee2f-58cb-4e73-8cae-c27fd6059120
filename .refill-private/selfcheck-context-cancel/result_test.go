package selfcheckcontextcancel

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"

	"strata-proof/internal/httpui"
)

func TestSelfCheckHonorsPreCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var calls atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "不应执行", http.StatusServiceUnavailable)
	})

	err := httpui.SelfCheck(ctx, "127.0.0.1:0", handler)
	if !errors.Is(err, context.Canceled) || calls.Load() != 0 {
		t.Fatalf("TestSelfCheckHonorsPreCanceledContext: 已取消 context 仍发起了 %d 次请求，返回 %v", calls.Load(), err)
	}
}
