// Package ws 实现 /api/ws：单连接多请求，每个请求独立 goroutine 并绑定可取消的 context。
package ws

import (
	"context"
	"encoding/json"
	"net"
	"sync"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"

	"github.com/chhongzh/scribekit/internal/agent"
	"github.com/chhongzh/scribekit/internal/agent/analyzer"
	"github.com/chhongzh/scribekit/internal/agent/scaffolder"
	"github.com/chhongzh/scribekit/internal/apperr"
	"github.com/chhongzh/scribekit/internal/logging"
	"github.com/chhongzh/scribekit/internal/store"
	"go.uber.org/zap"
)

const (
	textMessage = 1
	// 一条请求只带 id 和几个小字段，analyze.text 带一段正文，1 MiB 绰绰有余。
	maxMessageBytes = 1 << 20
	usageInterval   = 100 * time.Millisecond
)

type Analyzer interface {
	Paragraphs(ctx context.Context, articleID int, ids []int, emit func(analyzer.ParagraphAnalysis)) error
	Article(ctx context.Context, articleID int) (analyzer.ArticleAnalysis, error)
	Text(ctx context.Context, articleID int, paragraphKind, text string) (analyzer.ParagraphAnalysis, error)
}

type Scaffolder interface {
	Run(ctx context.Context, requestID string, req scaffolder.Request, ev scaffolder.Events) (store.Suggestion, error)
}

// Conn 是 Hub 对底层连接的最小依赖，测试里用内存实现替换。
type Conn interface {
	ReadMessage() (messageType int, p []byte, err error)
	WriteMessage(messageType int, data []byte) error
	Close() error
}

type Hub struct {
	analyzer   Analyzer
	scaffolder Scaffolder
	metricsMu  sync.Mutex
	metrics    agentMetrics
}

type agentMetrics struct {
	ActiveAgents     int `json:"activeAgents"`
	PromptTokens     int `json:"promptTokens"`
	CompletionTokens int `json:"completionTokens"`
	TotalTokens      int `json:"totalTokens"`
	MissingUsage     int `json:"missingUsage"`
}

func (h *Hub) recordUsage(usage *schema.TokenUsage) {
	h.metricsMu.Lock()
	defer h.metricsMu.Unlock()
	if usage == nil {
		h.metrics.MissingUsage++
		return
	}
	h.metrics.PromptTokens += usage.PromptTokens
	h.metrics.CompletionTokens += usage.CompletionTokens
	h.metrics.TotalTokens += usage.TotalTokens
}

func (h *Hub) recordActivity(delta int) {
	h.metricsMu.Lock()
	defer h.metricsMu.Unlock()
	h.metrics.ActiveAgents += delta
}

func (h *Hub) snapshotMetrics() agentMetrics {
	h.metricsMu.Lock()
	defer h.metricsMu.Unlock()
	return h.metrics
}

func NewHub(a Analyzer, s Scaffolder) *Hub { return &Hub{analyzer: a, scaffolder: s} }

// LocalOrigins 列出允许握手的 Origin：监听端口的回环地址，加上 Vite 开发服务器。
// 限制 Origin 是为了防止别的网页借浏览器连上本机服务，使用已保存的 API Key。
func LocalOrigins(addr string) []string {
	origins := []string{
		"http://localhost:5173", "http://127.0.0.1:5173",
	}
	if _, port, err := net.SplitHostPort(addr); err == nil && port != "" {
		origins = append(origins,
			"http://127.0.0.1:"+port, "http://localhost:"+port, "http://[::1]:"+port)
	}
	return origins
}

// Handler 返回挂在 /api/ws 上的 Fiber 处理器。
func (h *Hub) Handler(origins []string) fiber.Handler {
	return websocket.New(func(c *websocket.Conn) {
		c.SetReadLimit(maxMessageBytes)
		h.Serve(context.Background(), c)
	}, websocket.Config{Origins: origins, AllowEmptyOrigin: true})
}

type inbound struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type outbound struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

type session struct {
	hub  *Hub
	conn Conn
	ctx  context.Context

	wmu sync.Mutex

	mu       sync.Mutex
	inflight map[string]context.CancelFunc
	wg       sync.WaitGroup
}

// Serve 在当前 goroutine 里跑读循环，返回时已取消并等待全部在途请求结束。
func (h *Hub) Serve(parent context.Context, conn Conn) {
	ctx, cancel := context.WithCancel(parent)
	s := &session{hub: h, conn: conn, ctx: ctx, inflight: map[string]context.CancelFunc{}}
	usageDone := make(chan struct{})
	usageStopped := make(chan struct{})
	metrics := h.snapshotMetrics()
	s.send("", "agent.usage", metrics)
	go func() {
		defer close(usageStopped)
		ticker := time.NewTicker(usageInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				next := h.snapshotMetrics()
				if next != metrics {
					s.send("", "agent.usage", next)
					metrics = next
				}
			case <-usageDone:
				return
			}
		}
	}()
	defer func() {
		close(usageDone)
		cancel()
		_ = conn.Close()
		<-usageStopped
		s.wg.Wait()
	}()
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var env inbound
		if err := json.Unmarshal(data, &env); err != nil {
			s.sendError("", apperr.New(apperr.BadRequest, "消息不是合法的 JSON"))
			continue
		}
		s.dispatch(env)
	}
}

func (s *session) send(id, typ string, payload any) {
	b, err := json.Marshal(outbound{ID: id, Type: typ, Payload: payload})
	if err != nil {
		logging.From(s.ctx).Error("ws.marshal.failed", zap.String("request_id", id), zap.String("type", typ), zap.Error(err))
		return
	}
	s.wmu.Lock()
	defer s.wmu.Unlock()
	if err := s.conn.WriteMessage(textMessage, b); err != nil {
		logging.From(s.ctx).Warn("ws.write.failed", zap.String("request_id", id), zap.String("type", typ), zap.Error(err))
	}
}

func (s *session) sendError(id string, ae *apperr.Error) {
	s.send(id, "error", ae)
}

func (s *session) fail(id string, err error) {
	ae := apperr.From(err)
	if ae.Code == apperr.Internal {
		logging.From(s.ctx).Error("ws.request.failed", zap.String("request_id", id), zap.Error(err))
	}
	s.sendError(id, ae)
}

func (s *session) dispatch(env inbound) {
	switch env.Type {
	case "ping":
		s.send(env.ID, "pong", struct{}{})
	case "cancel":
		var p struct {
			TargetID string `json:"targetId"`
		}
		if err := json.Unmarshal(env.Payload, &p); err != nil || p.TargetID == "" {
			s.sendError(env.ID, apperr.New(apperr.BadRequest, "cancel 需要 targetId"))
			return
		}
		s.mu.Lock()
		cancel := s.inflight[p.TargetID]
		s.mu.Unlock()
		if cancel != nil {
			logging.From(s.ctx).Info("ws.cancel.requested", zap.String("request_id", p.TargetID))
			cancel()
		}
	case "analyze.paragraphs", "analyze.article", "analyze.text", "scaffold.request":
		s.start(env)
	default:
		s.sendError(env.ID, apperr.New(apperr.BadRequest, "未知消息类型 %q", env.Type))
	}
}

func (s *session) start(env inbound) {
	if env.ID == "" {
		s.sendError("", apperr.New(apperr.BadRequest, "请求缺少 id"))
		return
	}
	ctx, cancel := context.WithTimeout(s.ctx, 3*time.Minute)
	s.mu.Lock()
	if _, dup := s.inflight[env.ID]; dup {
		s.mu.Unlock()
		cancel()
		s.sendError(env.ID, apperr.New(apperr.BadRequest, "请求 id %q 正在处理中", env.ID))
		return
	}
	s.inflight[env.ID] = cancel
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() {
			s.mu.Lock()
			delete(s.inflight, env.ID)
			s.mu.Unlock()
			cancel()
		}()
		ctx, finish := logging.Start(ctx, "ws.ai.request", zap.String("request_id", env.ID), zap.String("request_type", env.Type))
		ctx = agent.WithUsageSink(ctx, s.hub.recordUsage)
		ctx = agent.WithActivitySink(ctx, s.hub.recordActivity)
		err := s.handle(ctx, env)
		finish(err)
		if err != nil {
			if s.ctx.Err() != nil {
				return
			}
			s.fail(env.ID, err)
		}
	}()
}

func decode[T any](raw json.RawMessage) (T, error) {
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		return v, apperr.New(apperr.BadRequest, "payload 格式不正确: %v", err)
	}
	return v, nil
}

func (s *session) handle(ctx context.Context, env inbound) error {
	switch env.Type {
	case "analyze.paragraphs":
		p, err := decode[struct {
			ArticleID    int   `json:"articleId"`
			ParagraphIDs []int `json:"paragraphIds"`
		}](env.Payload)
		if err != nil {
			return err
		}
		if err := s.hub.analyzer.Paragraphs(ctx, p.ArticleID, p.ParagraphIDs, func(a analyzer.ParagraphAnalysis) {
			s.send(env.ID, "analyze.result", a)
		}); err != nil {
			return err
		}
		s.send(env.ID, "analyze.done", struct{}{})
	case "analyze.article":
		p, err := decode[struct {
			ArticleID int `json:"articleId"`
		}](env.Payload)
		if err != nil {
			return err
		}
		res, err := s.hub.analyzer.Article(ctx, p.ArticleID)
		if err != nil {
			return err
		}
		s.send(env.ID, "analyze.articleResult", res)
	case "analyze.text":
		p, err := decode[struct {
			ArticleID     int    `json:"articleId"`
			ParagraphKind string `json:"paragraphKind"`
			Text          string `json:"text"`
		}](env.Payload)
		if err != nil {
			return err
		}
		res, err := s.hub.analyzer.Text(ctx, p.ArticleID, p.ParagraphKind, p.Text)
		if err != nil {
			return err
		}
		s.send(env.ID, "analyze.textResult", res)
	case "scaffold.request":
		req, err := decode[scaffolder.Request](env.Payload)
		if err != nil {
			return err
		}
		sg, err := s.hub.scaffolder.Run(ctx, env.ID, req, scaffolder.Events{
			Started: func(id int) {
				s.send(env.ID, "scaffold.started", map[string]int{"suggestionId": id})
			},
			Delta: func(id int, text string) {
				s.send(env.ID, "scaffold.delta", map[string]any{"suggestionId": id, "text": text})
			},
		})
		if err != nil {
			return err
		}
		s.send(env.ID, "scaffold.done", sg)
	}
	return nil
}
