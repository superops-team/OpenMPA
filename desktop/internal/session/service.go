package session

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/egoist/mygo"
	"github.com/superops-team/OpenMPA/desktop/internal/configstore"
	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Service struct {
	operationMu sync.Mutex
	conn        *connection.Client
	pending     *PendingStore
	cache       *configstore.Store
	traceMu     sync.Mutex
	traceIDs    map[string]bool
	traceTitles map[string]string
}
type runInput struct {
	Content                string          `json:"content"`
	ExecutionConfigVersion *int            `json:"executionConfigVersion,omitempty"`
	AgentsMD               string          `json:"agentsMd,omitempty"`
	BotProfile             *dto.BotProfile `json:"botProfile,omitempty"`
	Skills                 []string        `json:"skills,omitempty"`
}

func New(c *connection.Client, p *PendingStore) *Service {
	return &Service{conn: c, pending: p, traceIDs: map[string]bool{}, traceTitles: map[string]string{}}
}
func NewWithCache(c *connection.Client, p *PendingStore, cache *configstore.Store) *Service {
	service := New(c, p)
	service.cache = cache
	return service
}
func path(id, suffix string) string { return "/api/v1/sessions/" + url.PathEscape(id) + suffix }
func adminPath(id, suffix string) string {
	return "/api/v1/runtime-console/admin/sessions/" + url.PathEscape(id) + suffix
}
func (s *Service) assistantID(r dto.AssistantConfigRequest) string {
	if strings.TrimSpace(r.AssistantID) != "" {
		return strings.TrimSpace(r.AssistantID)
	}
	state := s.conn.State()
	if strings.TrimSpace(state.RuntimeID) != "" {
		return strings.TrimSpace(state.RuntimeID)
	}
	return strings.TrimSpace(r.SessionID)
}
func (s *Service) submitAssistantID(r dto.SubmitRequest) string {
	if strings.TrimSpace(r.AssistantID) != "" {
		return strings.TrimSpace(r.AssistantID)
	}
	state := s.conn.State()
	if strings.TrimSpace(state.RuntimeID) != "" {
		return strings.TrimSpace(state.RuntimeID)
	}
	return strings.TrimSpace(r.SessionID)
}
func selectedSkillIDs(skills []dto.AssistantSkill) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, skill := range skills {
		id := strings.TrimSpace(skill.ID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}
func cloneBotProfile(profile *dto.BotProfile) *dto.BotProfile {
	if profile == nil {
		return nil
	}
	out := *profile
	if profile.AgentsMD != nil {
		value := *profile.AgentsMD
		out.AgentsMD = &value
	}
	if profile.Model != nil {
		value := *profile.Model
		out.Model = &value
	}
	if profile.Skills != nil {
		value := append([]string(nil), (*profile.Skills)...)
		out.Skills = &value
	}
	if profile.MCP != nil {
		value := append([]string(nil), (*profile.MCP)...)
		out.MCP = &value
	}
	return &out
}
func applyBotProfileToRun(input *runInput, profile *dto.BotProfile) {
	if profile == nil {
		return
	}
	input.BotProfile = cloneBotProfile(profile)
	if profile.AgentsMD != nil {
		input.AgentsMD = *profile.AgentsMD
	}
	if profile.Skills != nil {
		input.Skills = append([]string(nil), (*profile.Skills)...)
	}
}
func (s *Service) assistantProfile(ctx context.Context, scope, assistantID string) *dto.BotProfile {
	if s.cache == nil || scope == "" || assistantID == "" {
		return nil
	}
	config, ok, err := s.cache.LoadAssistantConfig(ctx, scope, assistantID)
	if err != nil || !ok {
		return nil
	}
	profile := dto.BotProfile{}
	if strings.TrimSpace(config.AgentsMD) != "" {
		agents := config.AgentsMD
		profile.AgentsMD = &agents
	}
	if skills := selectedSkillIDs(config.Skills); len(skills) > 0 {
		profile.Skills = &skills
	}
	if profile.AgentsMD == nil && profile.Skills == nil {
		return nil
	}
	return &profile
}
func (s *Service) AssistantConfig(ctx context.Context, r dto.AssistantConfigRequest) dto.AssistantConfigResult {
	if !s.conn.Current(r.ConnectionEpoch) {
		return dto.AssistantConfigResult{Error: dto.Failure("staleConnection", "transport", "reload", 0)}
	}
	if s.cache == nil {
		return dto.AssistantConfigResult{Error: dto.Failure("localConfigUnavailable", "desktop", "never", 0)}
	}
	assistantID := s.assistantID(r)
	config, _, err := s.cache.LoadAssistantConfig(ctx, s.conn.State().Scope, assistantID)
	if err != nil {
		return dto.AssistantConfigResult{Error: dto.Failure("invalidInput", "desktop", "never", 0)}
	}
	return dto.AssistantConfigResult{OK: true, Value: &config}
}
func (s *Service) SaveAssistantConfig(ctx context.Context, r dto.AssistantConfigSaveRequest) dto.AssistantConfigResult {
	if !s.conn.Current(r.ConnectionEpoch) {
		return dto.AssistantConfigResult{Error: dto.Failure("staleConnection", "transport", "reload", 0)}
	}
	if s.cache == nil {
		return dto.AssistantConfigResult{Error: dto.Failure("localConfigUnavailable", "desktop", "never", 0)}
	}
	assistantID := s.assistantID(dto.AssistantConfigRequest{ConnectionEpoch: r.ConnectionEpoch, SessionID: r.SessionID, AssistantID: r.AssistantID})
	config, err := s.cache.SaveAssistantConfig(ctx, s.conn.State().Scope, dto.AssistantConfig{AssistantID: assistantID, AgentsMD: r.AgentsMD, Skills: r.Skills})
	if err != nil {
		return dto.AssistantConfigResult{Error: dto.Failure("invalidInput", "desktop", "never", 0)}
	}
	return dto.AssistantConfigResult{OK: true, Value: &config}
}
func (s *Service) SkillCatalog(ctx context.Context, r dto.SkillCatalogRequest) dto.SkillCatalogResult {
	if !s.conn.Current(r.ConnectionEpoch) {
		return dto.SkillCatalogResult{Error: dto.Failure("staleConnection", "transport", "reload", 0)}
	}
	catalog := []dto.AssistantSkill{
		{ID: "meegle", Name: "meegle", Description: "飞书项目（Meego）操作，支持查询管理工作项、节点流转、排期统计", Source: "skill-space", SpaceID: "agentkit"},
		{ID: "meegle-attention", Name: "meegle-attention", Description: "飞书项目待办提醒，自动汇总需要关注的项目、需求、缺陷和 deadline", Source: "skill-space", SpaceID: "agentkit"},
		{ID: "imagegen", Name: "imagegen", Description: "AI 图像生成编辑，支持生成位图、编辑现有图像、生成视觉变体", Source: "skill-space", SpaceID: "agentkit"},
		{ID: "openai-docs", Name: "openai-docs", Description: "OpenAI/Codex 官方文档查询，支持模型、API、技能、配置相关问题查询", Source: "skill-space", SpaceID: "agentkit"},
		{ID: "plugin-creator", Name: "plugin-creator", Description: "Codex 插件脚手架生成，支持创建插件目录、生成 manifest 配置", Source: "skill-space", SpaceID: "agentkit"},
		{ID: "skill-creator", Name: "skill-creator", Description: "Codex Skill 开发，支持创建或更新 Skill 指令及配套资源", Source: "skill-space", SpaceID: "agentkit"},
		{ID: "skill-installer", Name: "skill-installer", Description: "Codex Skill 安装，支持从官方列表或 GitHub 仓库安装 Skill", Source: "skill-space", SpaceID: "agentkit"},
		{ID: "data-analysis", Name: "data-analysis", Description: "本地数据分析，支持 CSV/JSONL/Parquet 数据清洗、聚合、统计", Source: "skill-space", SpaceID: "agentkit"},
	}
	query := strings.ToLower(strings.TrimSpace(r.Query))
	out := dto.SkillCatalog{Skills: []dto.AssistantSkill{}}
	for _, skill := range catalog {
		if query == "" || strings.Contains(strings.ToLower(skill.ID+" "+skill.Name+" "+skill.Description), query) {
			out.Skills = append(out.Skills, skill)
		}
	}
	return dto.SkillCatalogResult{OK: true, Value: &out}
}
func (s *Service) List(ctx context.Context, r dto.ScopeRequest) dto.SessionListResult {
	state := s.conn.State()
	if !s.conn.Current(r.ConnectionEpoch) {
		return dto.SessionListResult{Error: dto.Failure("staleConnection", "transport", "reload", 0)}
	}
	if !r.ForceRefresh && s.cache != nil {
		if cached, ok, err := s.cache.LoadSessionListCache(ctx, state.Scope, configstore.SessionListCacheTTL()); err == nil && ok {
			var cachedList dto.SessionList
			if json.Unmarshal(cached, &cachedList) == nil {
				for _, item := range cachedList.Sessions {
					id := item.ID
					if id == "" {
						id = item.SessionID
					}
					if item.TraceEnabled {
						s.markTraceSession(id)
						s.setTraceTitle(id, item.Title)
					}
				}
				return dto.SessionListResult{OK: true, Value: &cachedList}
			}
		}
	}
	out := dto.SessionList{Sessions: []dto.SessionSummary{}}
	e := s.conn.JSON(ctx, r.ConnectionEpoch, "GET", "/api/v1/sessions?include_a2a=false", nil, &out)
	if e == nil {
		for i := range out.Sessions {
			if out.Sessions[i].ID == "" {
				out.Sessions[i].ID = out.Sessions[i].SessionID
			}
		}
	}
	var admin dto.SessionList
	if ae := s.conn.JSON(ctx, r.ConnectionEpoch, "GET", "/api/v1/runtime-console/admin/sessions?limit=500", nil, &admin); ae == nil {
		seen := map[string]bool{}
		for _, item := range out.Sessions {
			id := item.ID
			if id == "" {
				id = item.SessionID
			}
			if id != "" {
				seen[id] = true
			}
		}
		for _, item := range admin.Sessions {
			if item.ID == "" {
				item.ID = item.SessionID
			}
			if item.ID == "" || seen[item.ID] {
				continue
			}
			item.TraceEnabled = true
			s.markTraceSession(item.ID)
			if title := s.traceTitle(item.ID); title != "" {
				item.Title = title
			}
			out.Sessions = append(out.Sessions, item)
		}
	}
	if e == nil && s.cache != nil {
		if data, err := json.Marshal(out); err == nil {
			_ = s.cache.SaveSessionListCache(ctx, state.Scope, data)
		}
	} else if e != nil && s.cache != nil {
		if cached, ok, err := s.cache.LoadSessionListCache(ctx, state.Scope, 7*24*time.Hour); err == nil && ok {
			var cachedList dto.SessionList
			if json.Unmarshal(cached, &cachedList) == nil {
				return dto.SessionListResult{OK: true, Value: &cachedList}
			}
		}
	}
	return dto.SessionListResult{OK: e == nil, Value: only(&out, e), Error: e}
}
func only[T any](v *T, e *dto.SafeError) *T {
	if e != nil {
		return nil
	}
	return v
}
func (s *Service) Create(ctx context.Context, r dto.CreateRequest) dto.SessionSummaryResult {
	agentID := strings.TrimSpace(r.MPAInstanceID)
	if agentID == "" {
		var overview struct {
			Agent struct {
				ID string `json:"id"`
			} `json:"agent"`
		}
		if e := s.conn.JSON(ctx, r.ConnectionEpoch, http.MethodGet, "/api/v1/runtime-console/overview", nil, &overview); e != nil {
			return dto.SessionSummaryResult{Error: e}
		}
		agentID = strings.TrimSpace(overview.Agent.ID)
	}
	if !validResourceID(agentID) {
		return dto.SessionSummaryResult{Error: dto.Failure("agentProfileUnavailable", "mpa", "never", 0)}
	}
	var created struct {
		SessionID               string `json:"sessionId"`
		CreatedAt               string `json:"createdAt"`
		MPAInstanceID           string `json:"mpaInstanceId"`
		ExecutionConfigRevision int    `json:"executionConfigRevision"`
	}
	input := dto.CreateRequest{ConnectionEpoch: r.ConnectionEpoch, SessionID: r.SessionID, MPAInstanceID: agentID}
	e := s.conn.JSON(ctx, r.ConnectionEpoch, http.MethodPost, "/api/v1/sessions", input, &created)
	if e != nil {
		return dto.SessionSummaryResult{Error: e}
	}
	if created.SessionID == "" || (r.SessionID != "" && created.SessionID != r.SessionID) {
		return dto.SessionSummaryResult{Error: dto.Failure("invalidResponse", "runtimeIngress", "verifyOnly", 0)}
	}
	out := dto.SessionSummary{ID: created.SessionID, SessionID: created.SessionID, Status: "idle", MPAInstanceID: agentID, CreatedAt: created.CreatedAt}
	return dto.SessionSummaryResult{OK: true, Value: &out}
}

// CreateTraceSession is an explicit diagnostics operation. Product
// conversations use Create so their next-Turn model remains configurable.
func (s *Service) CreateTraceSession(ctx context.Context, r dto.CreateRequest) dto.SessionSummaryResult {
	var out dto.SessionSummary
	e := s.conn.JSON(ctx, r.ConnectionEpoch, "POST", "/api/v1/runtime-console/admin/debug-sessions", nil, &out)
	if out.ID == "" {
		out.ID = out.SessionID
	}
	out.TraceEnabled = e == nil
	if e == nil {
		s.markTraceSession(out.ID)
	}
	return dto.SessionSummaryResult{OK: e == nil, Value: only(&out, e), Error: e}
}
func (s *Service) Rename(ctx context.Context, r dto.RenameRequest) dto.SessionSummaryResult {
	var out dto.SessionSummary
	if s.traceSession(r.SessionID) {
		s.setTraceTitle(r.SessionID, r.Title)
		out = dto.SessionSummary{ID: r.SessionID, SessionID: r.SessionID, Title: r.Title, Status: "idle", TraceEnabled: true}
		return dto.SessionSummaryResult{OK: true, Value: &out}
	}
	e := s.conn.JSON(ctx, r.ConnectionEpoch, "PUT", path(r.SessionID, ""), struct {
		Title string `json:"title"`
	}{r.Title}, &out)
	if out.ID == "" {
		out.ID = out.SessionID
	}
	return dto.SessionSummaryResult{OK: e == nil, Value: only(&out, e), Error: e}
}
func (s *Service) Delete(ctx context.Context, r dto.SessionRequest) dto.DeleteReceiptResult {
	var out dto.DeleteReceipt
	deletePath := path(r.SessionID, "")
	if s.traceSession(r.SessionID) {
		deletePath = adminPath(r.SessionID, "")
	}
	e := s.conn.JSON(ctx, r.ConnectionEpoch, "DELETE", deletePath, nil, &out)
	return dto.DeleteReceiptResult{OK: e == nil, Value: only(&out, e), Error: e}
}
func (s *Service) Submit(ctx context.Context, r dto.SubmitRequest) dto.RunReceiptResult {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	if len(r.AttachmentRefs) != 0 || r.OperationID == "" || r.IdempotencyKey == "" || r.Content == "" || len(r.Content) > 4<<20 || strings.ContainsAny(r.IdempotencyKey, "\r\n") {
		return dto.RunReceiptResult{Error: dto.Failure("invalidInput", "mpa", "never", 0)}
	}
	input := runInput{Content: r.Content}
	if r.ExecutionConfigVersion != "" {
		v, err := strconv.Atoi(r.ExecutionConfigVersion)
		if err != nil || v < 1 {
			return dto.RunReceiptResult{Error: dto.Failure("invalidInput", "mpa", "never", 0)}
		}
		input.ExecutionConfigVersion = &v
	}
	state := s.conn.State()
	if !s.conn.Current(r.ConnectionEpoch) {
		return dto.RunReceiptResult{Error: dto.Failure("staleConnection", "transport", "reload", 0)}
	}
	operations, pendingError := s.pending.List(state.Scope)
	if pendingError != nil {
		return dto.RunReceiptResult{Error: dto.Failure("pendingPersistenceFailed", "transport", "never", 0)}
	}
	assistantID := s.submitAssistantID(r)
	profile := s.assistantProfile(ctx, state.Scope, assistantID)
	applyBotProfileToRun(&input, profile)
	for _, operation := range operations {
		if operation.OperationID == r.OperationID || operation.IdempotencyKey == r.IdempotencyKey {
			if operation.SessionID != r.SessionID || operation.Content != r.Content || operation.ConfigVersion != r.ExecutionConfigVersion || operation.AssistantID != assistantID || !equalBotProfile(operation.AssistantProfile, profile) || !equalAttachmentRefs(operation.AttachmentRefs, r.AttachmentRefs) {
				return dto.RunReceiptResult{Error: dto.Failure("idempotency_mismatch", "transport", "never", 409)}
			}
		}
		if operation.OperationID == r.OperationID || operation.SessionID == r.SessionID {
			return dto.RunReceiptResult{Error: dto.Failure("outcomeUnknown", "transport", "verifyOnly", 0)}
		}
	}
	traceEnabled := s.traceSession(r.SessionID)
	p := Pending{AttachmentRefs: cloneAttachmentRefs(r.AttachmentRefs), AssistantID: assistantID, AssistantProfile: cloneBotProfile(profile), Scope: state.Scope, SessionID: r.SessionID, OperationID: r.OperationID, IdempotencyKey: r.IdempotencyKey, Content: r.Content, ConfigVersion: r.ExecutionConfigVersion, DurableBot: s.durableBot(ctx, r.ConnectionEpoch, r.SessionID, r.ExecutionConfigVersion), TraceEnabled: traceEnabled}
	if e := s.pending.Put(p); e != nil {
		if errors.Is(e, ErrPendingExists) {
			return dto.RunReceiptResult{Error: dto.Failure("outcomeUnknown", "transport", "verifyOnly", 0)}
		}
		return dto.RunReceiptResult{Error: dto.Failure("pendingPersistenceFailed", "transport", "never", 0)}
	}

	data, _ := json.Marshal(input)
	headers := http.Header{}
	headers.Set("Idempotency-Key", r.IdempotencyKey)
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	runPath := path(r.SessionID, "/run")
	if traceEnabled {
		runPath = adminPath(r.SessionID, "/run")
	}
	resp, e := s.conn.Request(ctx, r.ConnectionEpoch, "POST", runPath, data, headers)
	var out dto.RunReceipt
	if e == nil {
		defer resp.Body.Close()
		if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out) != nil || out.InvocationID == "" {
			e = dto.Failure("outcomeUnknown", "runtimeIngress", "verifyOnly", resp.StatusCode)
		}
	}
	if e == nil {
		if err := s.pending.Update(state.Scope, r.OperationID, func(p *Pending) error { p.State = "accepted"; p.InvocationID = out.InvocationID; return nil }); err != nil {
			e = dto.Failure("pendingPersistenceFailed", "transport", "verifyOnly", 0)
		}
	}

	return dto.RunReceiptResult{OK: e == nil, Value: only(&out, e), Error: e}
}
func (s *Service) Subscribe(ctx context.Context, r dto.SubscribeRequest, channel *mygo.Channel[dto.ClientEvent]) dto.EmptyResult {
	return s.stream(ctx, r, channel.Send)
}
func (s *Service) stream(ctx context.Context, r dto.SubscribeRequest, send func(dto.ClientEvent) error) dto.EmptyResult {
	input := struct {
		LastEventID  string `json:"lastEventId,omitempty"`
		InvocationID string `json:"invocationId"`
	}{r.LastEventID, r.InvocationID}
	body, _ := json.Marshal(input)
	streamPath := path(r.SessionID, "/sse")
	if s.traceSession(r.SessionID) {
		streamPath = adminPath(r.SessionID, "/sse")
	}
	resp, e := s.conn.Request(ctx, r.ConnectionEpoch, "POST", streamPath, body, nil)
	if e != nil {
		_ = send(dto.ClientEvent{SchemaVersion: 1, Kind: "error", ConnectionEpoch: r.ConnectionEpoch, SubscriptionEpoch: r.SubscriptionEpoch, SessionID: r.SessionID, InvocationID: r.InvocationID, Error: e})
		return dto.EmptyResult{Error: e}
	}
	defer resp.Body.Close()
	err := ParseSSE(resp.Body, r, func(event dto.ClientEvent) error {
		if !s.conn.Current(r.ConnectionEpoch) {
			return context.Canceled
		}
		if event.Terminal && (event.Kind == "done" || event.Kind == "error") && event.InvocationID == r.InvocationID {
			s.observeTerminal(ctx, r)
		}
		return send(event)
	})
	if err != nil && ctx.Err() == nil && s.conn.Current(r.ConnectionEpoch) {
		e = dto.Failure("streamInterrupted", "runtimeIngress", "history", 0)
		_ = send(dto.ClientEvent{SchemaVersion: 1, Kind: "error", ConnectionEpoch: r.ConnectionEpoch, SubscriptionEpoch: r.SubscriptionEpoch, SessionID: r.SessionID, InvocationID: r.InvocationID, Error: e})
		return dto.EmptyResult{Error: e}
	}
	return dto.EmptyResult{OK: true, Value: &dto.Empty{}}
}
func (s *Service) Abort(ctx context.Context, r dto.SessionRequest) dto.AbortReceiptResult {
	var out dto.AbortReceipt
	abortPath := path(r.SessionID, "/abort")
	if s.traceSession(r.SessionID) {
		abortPath = adminPath(r.SessionID, "/abort")
	}
	e := s.conn.JSON(ctx, r.ConnectionEpoch, "POST", abortPath, nil, &out)
	return dto.AbortReceiptResult{OK: e == nil, Value: only(&out, e), Error: e}
}
func (s *Service) ExecutionConfig(ctx context.Context, r dto.SessionRequest) dto.ExecutionConfigResult {
	resp, e := s.conn.Request(ctx, r.ConnectionEpoch, "GET", path(r.SessionID, "/execution-config"), nil, nil)
	if e != nil {
		return dto.ExecutionConfigResult{Error: e}
	}
	return decodeExecutionConfig(resp)
}
func (s *Service) Pending(ctx context.Context, r dto.ScopeRequest) dto.PendingListResult {
	if !s.conn.Current(r.ConnectionEpoch) {
		return dto.PendingListResult{Error: dto.Failure("staleConnection", "transport", "reload", 0)}
	}
	list, e := s.pending.List(s.conn.State().Scope)
	if e != nil {
		return dto.PendingListResult{Error: dto.Failure("pendingPersistenceFailed", "transport", "never", 0)}
	}
	out := dto.PendingList{Operations: []dto.PendingSummary{}}
	for _, p := range list {
		if p.TraceEnabled {
			s.markTraceSession(p.SessionID)
		}
		out.Operations = append(out.Operations, p.Summary())
	}
	return dto.PendingListResult{OK: true, Value: &out}
}
