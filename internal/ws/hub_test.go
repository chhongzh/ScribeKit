package ws

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
)

type metricsConn struct {
	writes chan []byte
	closed chan struct{}
	once   sync.Once
}

func (conn *metricsConn) ReadMessage() (int, []byte, error) {
	<-conn.closed
	return 0, nil, io.EOF
}

func (conn *metricsConn) WriteMessage(_ int, data []byte) error {
	select {
	case conn.writes <- data:
		return nil
	case <-conn.closed:
		return io.EOF
	}
}

func (conn *metricsConn) Close() error {
	conn.once.Do(func() { close(conn.closed) })
	return nil
}

func startMetricsSession(t *testing.T, hub *Hub) *metricsConn {
	t.Helper()
	conn := &metricsConn{writes: make(chan []byte, 16), closed: make(chan struct{})}
	done := make(chan struct{})
	go func() {
		hub.Serve(context.Background(), conn)
		close(done)
	}()
	t.Cleanup(func() {
		conn.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("metrics session did not close")
		}
	})
	return conn
}

func receiveMetrics(t *testing.T, conn *metricsConn) agentMetrics {
	t.Helper()
	select {
	case data := <-conn.writes:
		var message struct {
			Type    string       `json:"type"`
			Payload agentMetrics `json:"payload"`
		}
		if err := json.Unmarshal(data, &message); err != nil || message.Type != "agent.usage" {
			t.Fatalf("invalid metrics frame: %s, %v", data, err)
		}
		return message.Payload
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for metrics")
		return agentMetrics{}
	}
}

func TestMetricsThrottleLiveUsageAndPreserveTotalsAcrossConnections(t *testing.T) {
	hub := NewHub(nil, nil)
	conn := startMetricsSession(t, hub)
	if metrics := receiveMetrics(t, conn); metrics != (agentMetrics{}) {
		t.Fatalf("unexpected initial metrics: %+v", metrics)
	}
	hub.recordActivity(1)
	for range 100 {
		hub.recordUsage(&schema.TokenUsage{CompletionTokens: 1, TotalTokens: 1})
	}
	metrics := receiveMetrics(t, conn)
	if metrics.ActiveAgents != 1 || metrics.CompletionTokens != 100 || metrics.TotalTokens != 100 {
		t.Fatalf("stream deltas were not coalesced while the agent was active: %+v", metrics)
	}
	select {
	case data := <-conn.writes:
		t.Fatalf("unchanged usage should not be pushed again: %s", data)
	case <-time.After(2 * usageInterval):
	}
	hub.recordUsage(nil)
	hub.recordActivity(-1)
	metrics = receiveMetrics(t, conn)
	if metrics.ActiveAgents != 0 || metrics.TotalTokens != 100 || metrics.MissingUsage != 1 {
		t.Fatalf("final metrics were not pushed: %+v", metrics)
	}
	reconnected := startMetricsSession(t, hub)
	if initial := receiveMetrics(t, reconnected); initial != metrics {
		t.Fatalf("reconnecting changed cumulative totals: before=%+v after=%+v", metrics, initial)
	}
}
