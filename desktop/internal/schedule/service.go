package schedule

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

const taskPath = "/api/v1/esa-cron-tasks"

type connectionProvider interface {
	Do(context.Context, uint64, string, string, interface{}, interface{}, string) *dto.SafeError
	State() dto.ConnectionState
	Current(uint64) bool
}
type Service struct{ conn connectionProvider }

// New does not read credentials or initiate network activity during MyGo binding.
func New(conn *connection.Client) *Service {
	if conn == nil {
		return &Service{}
	}
	return &Service{conn: conn}
}

func invalidInput() *dto.SafeError { return dto.Failure("invalidInput", "schedule", "never", 0) }
func invalidResponse() *dto.SafeError {
	return dto.Failure("invalidResponse", "schedule", "verifyOnly", 200)
}
func (s *Service) ready(epoch uint64) *dto.SafeError {
	if s.conn == nil || !s.conn.State().Connected {
		return dto.Failure("notConnected", "transport", "never", 0)
	}
	if !s.conn.Current(epoch) {
		return dto.Failure("staleConnection", "transport", "reload", 0)
	}
	return nil
}
func (s *Service) do(epoch uint64, method, path string, input, output interface{}) *dto.SafeError {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return s.doContext(ctx, epoch, method, path, input, output)
}
func (s *Service) doContext(ctx context.Context, epoch uint64, method, path string, input, output interface{}) *dto.SafeError {
	if e := s.ready(epoch); e != nil {
		return e
	}
	e := s.conn.Do(ctx, epoch, method, path, input, output, "schedule")
	if e != nil {
		// Never mutate a shared provider error or automatically retry a write.
		copy := *e
		if method != http.MethodGet && (e.HTTPStatus >= 500 || e.Code == "outcomeUnknown" || e.Code == "invalidResponse" || e.Code == "staleConnection") {
			copy.RetryDisposition = "verifyOnly"
		}
		return &copy
	}
	if !s.conn.Current(epoch) {
		retry := "reload"
		if method != http.MethodGet {
			retry = "verifyOnly"
		}
		return dto.Failure("staleConnection", "transport", retry, 0)
	}
	return nil
}
func limitDefault(limit int) int {
	if limit == 0 {
		return 20
	}
	return limit
}
func countAvailable(o *Overview) bool {
	return o != nil && o.ExecutionCount != nil && *o.ExecutionCount >= 0
}

// The deployed schema has no availability flag. If a newer server explicitly
// supplies one, false wins over any returned counters; absence remains distinct.
type taskPageWire struct {
	TaskPage
	CountAvailable *bool `json:"countAvailable"`
}
type runPageWire struct {
	RunPage
	CountAvailable *bool `json:"countAvailable"`
}

func cleanRun(r *TaskRun) {
	if r.ErrorCode != nil && !errorCodePattern.MatchString(*r.ErrorCode) {
		r.ErrorCode = nil
	}
}

func (s *Service) List(r ListRequest) ListResult {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return s.list(ctx, r)
}

// ListForAgent reads the bounded task directory and filters by the assistant
// that owns each task. The deployed collection route has no agentId filter, so
// filtering only the first page would silently hide tasks from later pages.
func (s *Service) ListForAgent(r AgentListRequest) ListResult {
	if !textValid(r.AgentID, 256) || !utf8.ValidString(r.Query) || len([]rune(r.Query)) > 256 {
		return ListResult{Error: invalidInput()}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	const pageSize = 200
	const maxTasks = 1000
	offset := 0
	items := []Task{}
	for pages := 0; pages < 20; pages++ {
		if ctx.Err() != nil {
			return ListResult{Error: dto.Failure("unavailable", "schedule", "readOnly", 0)}
		}
		result := s.list(ctx, ListRequest{ConnectionEpoch: r.ConnectionEpoch, IncludeDisabled: r.IncludeDisabled, Query: r.Query, SortBy: "nextRunAt", SortDirection: "Asc", Limit: pageSize, Offset: offset})
		if result.Error != nil {
			return result
		}
		page := result.Value
		for _, task := range page.Items {
			if task.AgentID == r.AgentID {
				items = append(items, task)
				if len(items) > maxTasks {
					return ListResult{Error: dto.Failure("boundedResult", "schedule", "readOnly", 0)}
				}
			}
		}
		if page.HasMore == nil {
			return ListResult{Error: invalidResponse()}
		}
		if !*page.HasMore {
			break
		}
		if page.NextOffset == nil || *page.NextOffset <= offset {
			return ListResult{Error: invalidResponse()}
		}
		offset = *page.NextOffset
	}
	return ListResult{OK: true, Value: &TaskPage{Items: items, Limit: pageSize, Offset: 0, CountAvailable: false}}
}
func (s *Service) list(ctx context.Context, r ListRequest) ListResult {
	if !pageValid(r.Limit, r.Offset) || !utf8.ValidString(r.Query) || len([]rune(r.Query)) > 256 {
		return ListResult{Error: invalidInput()}
	}
	if r.SortBy == "" {
		r.SortBy = "nextRunAt"
	}
	if r.SortDirection == "" {
		r.SortDirection = "Asc"
	}
	if (r.SortBy != "nextRunAt" && r.SortBy != "updatedAt" && r.SortBy != "name") || (r.SortDirection != "Asc" && r.SortDirection != "Desc") {
		return ListResult{Error: invalidInput()}
	}
	include := true
	if r.IncludeDisabled != nil {
		include = *r.IncludeDisabled
	}
	q := url.Values{"limit": {strconv.Itoa(limitDefault(r.Limit))}, "offset": {strconv.Itoa(r.Offset)}, "includeDisabled": {strconv.FormatBool(include)}, "query": {r.Query}, "sortBy": {r.SortBy}, "sortDirection": {r.SortDirection}}
	var wire taskPageWire
	if e := s.doContext(ctx, r.ConnectionEpoch, http.MethodGet, taskPath+"?"+q.Encode(), nil, &wire); e != nil {
		return ListResult{Error: e}
	}
	page := wire.TaskPage
	if page.Items == nil || (page.Total != nil && *page.Total < 0) {
		return ListResult{Error: invalidResponse()}
	}
	page.CountAvailable = countAvailable(page.Overview) && (wire.CountAvailable == nil || *wire.CountAvailable)
	return ListResult{OK: true, Value: &page}
}

// Read uses the authoritative collection route; the deployed API has no GET /id.
// Pagination is bounded by a shared timeout and requires an advancing server cursor.
func (s *Service) Read(r TaskRequest) TaskResult {
	if !idPattern.MatchString(r.TaskID) {
		return TaskResult{Error: invalidInput()}
	}
	if e := s.ready(r.ConnectionEpoch); e != nil {
		return TaskResult{Error: e}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	offset := 0
	for pages := 0; pages < 100; pages++ {
		if ctx.Err() != nil {
			return TaskResult{Error: dto.Failure("unavailable", "schedule", "readOnly", 0)}
		}
		result := s.list(ctx, ListRequest{ConnectionEpoch: r.ConnectionEpoch, IncludeDisabled: boolValue(true), SortBy: "name", SortDirection: "Asc", Limit: 200, Offset: offset})
		if result.Error != nil {
			return TaskResult{Error: result.Error}
		}
		for _, task := range result.Value.Items {
			if task.ID == r.TaskID {
				return TaskResult{OK: true, Value: &TaskValue{Task: task}}
			}
		}
		page := result.Value
		if page.HasMore == nil {
			return TaskResult{Error: invalidResponse()}
		}
		if !*page.HasMore {
			return TaskResult{Error: dto.Failure("ScheduledTaskNotFound", "schedule", "never", 404)}
		}
		if page.NextOffset == nil || *page.NextOffset <= offset {
			return TaskResult{Error: invalidResponse()}
		}
		offset = *page.NextOffset
	}
	return TaskResult{Error: dto.Failure("unavailable", "schedule", "readOnly", 0)}
}
func boolValue(v bool) *bool { return &v }

type createPayload struct {
	ClientToken    string    `json:"clientToken"`
	Name           string    `json:"name"`
	AgentID        string    `json:"agentId,omitempty"`
	Prompt         string    `json:"prompt"`
	Enabled        *bool     `json:"enabled,omitempty"`
	Schedule       Config    `json:"schedule"`
	Delivery       *Delivery `json:"delivery,omitempty"`
	JitterSeconds  *int      `json:"jitterSeconds,omitempty"`
	TimeoutSeconds *int      `json:"timeoutSeconds,omitempty"`
}

func (s *Service) Create(r CreateRequest) CreateResult {
	result := CreateResult{ClientToken: r.ClientToken}
	if !textValid(r.ClientToken, 128) || !textValid(r.Name, 128) || !textValid(r.Prompt, 4000) || (r.AgentID != "" && !textValid(r.AgentID, 256)) || !secondsValid(r.JitterSeconds, r.TimeoutSeconds) {
		result.Error = invalidInput()
		return result
	}
	config, valid := normalizeConfig(r.Schedule)
	if !valid {
		result.Error = invalidInput()
		return result
	}
	var delivery *Delivery
	if r.Delivery != nil {
		d, ok := normalizeDelivery(*r.Delivery)
		if !ok {
			result.Error = invalidInput()
			return result
		}
		delivery = &d
	}
	payload := createPayload{ClientToken: r.ClientToken, Name: r.Name, AgentID: r.AgentID, Prompt: r.Prompt, Enabled: r.Enabled, Schedule: config, Delivery: delivery, JitterSeconds: r.JitterSeconds, TimeoutSeconds: r.TimeoutSeconds}
	var value TaskValue
	if e := s.do(r.ConnectionEpoch, http.MethodPost, taskPath, payload, &value); e != nil {
		result.Error = e
		return result
	}
	if value.Task.ID == "" || value.Task.Version < 1 {
		result.Error = invalidResponse()
		return result
	}
	result.OK = true
	result.Value = &value
	return result
}

type updatePayload struct {
	ExpectedVersion int       `json:"expectedVersion"`
	Name            *string   `json:"name,omitempty"`
	AgentID         *string   `json:"agentId,omitempty"`
	Prompt          *string   `json:"prompt,omitempty"`
	Enabled         *bool     `json:"enabled,omitempty"`
	Schedule        *Config   `json:"schedule,omitempty"`
	Delivery        *Delivery `json:"delivery,omitempty"`
	JitterSeconds   *int      `json:"jitterSeconds,omitempty"`
	TimeoutSeconds  *int      `json:"timeoutSeconds,omitempty"`
}

func (s *Service) Update(r UpdateRequest) TaskResult {
	if !idPattern.MatchString(r.TaskID) || r.ExpectedVersion < 1 || !secondsValid(r.JitterSeconds, r.TimeoutSeconds) || (r.Name != nil && !textValid(*r.Name, 128)) || (r.AgentID != nil && !textValid(*r.AgentID, 256)) || (r.Prompt != nil && !textValid(*r.Prompt, 4000)) {
		return TaskResult{Error: invalidInput()}
	}
	var config *Config
	if r.Schedule != nil {
		c, valid := normalizeConfig(*r.Schedule)
		if !valid {
			return TaskResult{Error: invalidInput()}
		}
		config = &c
	}
	var delivery *Delivery
	if r.Delivery != nil {
		d, valid := normalizeDelivery(*r.Delivery)
		if !valid {
			return TaskResult{Error: invalidInput()}
		}
		delivery = &d
	}
	payload := updatePayload{ExpectedVersion: r.ExpectedVersion, Name: r.Name, AgentID: r.AgentID, Prompt: r.Prompt, Enabled: r.Enabled, Schedule: config, Delivery: delivery, JitterSeconds: r.JitterSeconds, TimeoutSeconds: r.TimeoutSeconds}
	var value TaskValue
	if e := s.do(r.ConnectionEpoch, http.MethodPost, taskPath+"/"+r.TaskID, payload, &value); e != nil {
		return TaskResult{Error: e}
	}
	if value.Task.ID != r.TaskID || value.Task.Version < 1 {
		return TaskResult{Error: invalidResponse()}
	}
	return TaskResult{OK: true, Value: &value}
}

type runPayload struct {
	ClientToken string `json:"clientToken"`
	Mode        string `json:"mode"`
}

func (s *Service) Run(r RunRequest) RunResult {
	result := RunResult{ClientToken: r.ClientToken}
	if r.Mode == "" {
		r.Mode = "Force"
	}
	if !idPattern.MatchString(r.TaskID) || !textValid(r.ClientToken, 128) || r.Mode != "Force" {
		result.Error = invalidInput()
		return result
	}
	var value RunValue
	if e := s.do(r.ConnectionEpoch, http.MethodPost, taskPath+"/"+r.TaskID+"/run", runPayload{ClientToken: r.ClientToken, Mode: r.Mode}, &value); e != nil {
		result.Error = e
		return result
	}
	if value.Run.ID == "" || value.Run.TaskID != r.TaskID {
		result.Error = invalidResponse()
		return result
	}
	cleanRun(&value.Run)
	result.OK = true
	result.Value = &value
	return result
}

func (s *Service) Runs(r RunsRequest) RunsResult {
	if !idPattern.MatchString(r.TaskID) || !pageValid(r.Limit, r.Offset) {
		return RunsResult{Error: invalidInput()}
	}
	q := url.Values{"limit": {strconv.Itoa(limitDefault(r.Limit))}, "offset": {strconv.Itoa(r.Offset)}}
	var wire runPageWire
	if e := s.do(r.ConnectionEpoch, http.MethodGet, taskPath+"/"+r.TaskID+"/runs?"+q.Encode(), nil, &wire); e != nil {
		return RunsResult{Error: e}
	}
	page := wire.RunPage
	if page.Items == nil || (page.Total != nil && *page.Total < 0) {
		return RunsResult{Error: invalidResponse()}
	}
	for i := range page.Items {
		if page.Items[i].TaskID != r.TaskID {
			return RunsResult{Error: invalidResponse()}
		}
		cleanRun(&page.Items[i])
	}
	page.CountAvailable = countAvailable(page.Overview) && (wire.CountAvailable == nil || *wire.CountAvailable)
	return RunsResult{OK: true, Value: &page}
}

// Deleting a definition removes future dispatches. This receipt makes no claim
// about the state of executions that were already accepted by the server.
func (s *Service) Delete(r TaskRequest) DeleteResult {
	if !idPattern.MatchString(r.TaskID) {
		return DeleteResult{Error: invalidInput()}
	}
	var wire struct {
		Removed *bool `json:"removed"`
	}
	if e := s.do(r.ConnectionEpoch, http.MethodDelete, taskPath+"/"+r.TaskID, nil, &wire); e != nil {
		return DeleteResult{Error: e}
	}
	if wire.Removed == nil {
		return DeleteResult{Error: invalidResponse()}
	}
	value := DeleteValue{TaskID: r.TaskID, Removed: *wire.Removed}
	return DeleteResult{OK: true, Value: &value}
}

func (s *Service) DeliveryTargets(r DeliveryTargetsRequest) DeliveryTargetsResult {
	if r.Channel == "" {
		r.Channel = "Feishu"
	}
	if r.Channel != "Feishu" || (r.AppID != "" && !textValid(r.AppID, 256)) {
		return DeliveryTargetsResult{Error: invalidInput()}
	}
	q := url.Values{"channel": {r.Channel}}
	if r.AppID != "" {
		q.Set("appId", strings.TrimSpace(r.AppID))
	}
	var value DeliveryTargetPage
	if e := s.do(r.ConnectionEpoch, http.MethodGet, taskPath+"/delivery-targets?"+q.Encode(), nil, &value); e != nil {
		return DeliveryTargetsResult{Error: e}
	}
	if value.Items == nil {
		return DeliveryTargetsResult{Error: invalidResponse()}
	}
	return DeliveryTargetsResult{OK: true, Value: &value}
}
