package schedule

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

type call struct {
	method, path, family string
	epoch                uint64
	body                 []byte
}
type recordingConnection struct {
	calls     []call
	responses []string
	errors    []*dto.SafeError
	current   bool
	deadlines []time.Time
}

func (r *recordingConnection) State() dto.ConnectionState {
	return dto.ConnectionState{Connected: true, ConnectionEpoch: 7, HasBusinessIdentity: true}
}
func (r *recordingConnection) Current(epoch uint64) bool { return r.current && epoch == 7 }
func (r *recordingConnection) Do(ctx context.Context, epoch uint64, method, path string, input, output interface{}, family string) *dto.SafeError {
	if deadline, ok := ctx.Deadline(); ok {
		r.deadlines = append(r.deadlines, deadline)
	}
	body, _ := json.Marshal(input)
	r.calls = append(r.calls, call{method: method, path: path, family: family, epoch: epoch, body: body})
	if len(r.errors) > 0 {
		e := r.errors[0]
		r.errors = r.errors[1:]
		if e != nil {
			return e
		}
	}
	response := `{}`
	if len(r.responses) > 0 {
		response = r.responses[0]
		r.responses = r.responses[1:]
	}
	if err := json.Unmarshal([]byte(response), output); err != nil {
		return dto.Failure("invalidResponse", "schedule", "verifyOnly", 200)
	}
	return nil
}
func serviceFor(responses ...string) (*Service, *recordingConnection) {
	r := &recordingConnection{responses: responses, current: true}
	return &Service{conn: r}, r
}
func intPtr(v int) *int       { return &v }
func boolPtr(v bool) *bool    { return &v }
func strPtr(v string) *string { return &v }

const taskWire = `{"task":{"id":"task-1","version":3,"name":"Check","agentId":"mpa","prompt":"Check status","enabled":true,"schedule":{"type":"Daily","timezone":"Asia/Shanghai","time":"08:00"},"lastError":"Bearer secret","createdAt":"2026-10-25T00:00:00Z","updatedAt":"2026-10-25T00:00:00Z"}}`

func TestCreateSchemaVariants(t *testing.T) {
	cases := []Config{
		{Type: "Once", RunAt: "2026-10-25T19:00:00+08:00"},
		{Type: "Interval", IntervalSeconds: intPtr(30), AnchorAt: "2026-10-25T00:00:00Z"},
		{Type: "Daily", Time: "08:00"},
		{Type: "Weekly", Time: "08:00", Weekdays: []int{7, 1}},
		{Type: "Monthly", Time: "08:00", MonthDays: []int{31, 1}},
		{Type: "Cron", CronExpression: "0 0 9 ? * MON-FRI"},
	}
	for _, config := range cases {
		t.Run(config.Type, func(t *testing.T) {
			s, r := serviceFor(taskWire)
			result := s.Create(CreateRequest{ConnectionEpoch: 7, ClientToken: "create-token", Name: "Check", Prompt: "Check status", Schedule: config, Enabled: boolPtr(false)})
			if !result.OK || result.Value == nil || result.Error != nil {
				t.Fatalf("create failed: %+v", result)
			}
			if result.ClientToken != "create-token" || len(r.calls) != 1 {
				t.Fatalf("idempotency changed: %+v", result)
			}
			c := r.calls[0]
			if c.method != "POST" || c.path != "/api/v1/esa-cron-tasks" || c.family != "schedule" || c.epoch != 7 {
				t.Fatalf("wrong route: %+v", c)
			}
			var wire map[string]json.RawMessage
			if e := json.Unmarshal(c.body, &wire); e != nil {
				t.Fatal(e)
			}
			if string(wire["enabled"]) != "false" {
				t.Fatal("explicit false must survive")
			}
			for _, key := range []string{"connectionEpoch", "taskId", "userId", "timezone", "intervalSeconds"} {
				if _, exists := wire[key]; exists {
					t.Fatalf("IPC metadata escaped into API: %s", key)
				}
			}
			var encoded Config
			if e := json.Unmarshal(wire["schedule"], &encoded); e != nil {
				t.Fatal(e)
			}
			if encoded.Type != config.Type || encoded.Timezone != "Asia/Shanghai" {
				t.Fatalf("wrong schedule: %+v", encoded)
			}
			if config.Type == "Cron" && encoded.CronExpression != "0 9 * * MON-FRI" {
				t.Fatalf("incorrect cron normalization: %q", encoded.CronExpression)
			}
			encodedResult, _ := json.Marshal(result)
			if strings.Contains(string(encodedResult), "Bearer secret") {
				t.Fatal("unsafe upstream error text crossed IPC")
			}
		})
	}
}

func TestSchemaValidationPreventsNetwork(t *testing.T) {
	bad := []Config{
		{Type: "Once", RunAt: "2026-10-25T12:00:00"},
		{Type: "Interval", IntervalSeconds: intPtr(29)},
		{Type: "Interval", IntervalSeconds: intPtr(31536001)},
		{Type: "Daily", Time: "25:00"},
		{Type: "Daily", Time: "08:00", IntervalSeconds: intPtr(60)},
		{Type: "Weekly", Time: "08:00", Weekdays: []int{1, 1}},
		{Type: "Weekly", Time: "08:00", Weekdays: []int{0}},
		{Type: "Monthly", Time: "08:00", MonthDays: []int{32}},
		{Type: "Cron", CronExpression: "0 0 9 ? * 2"},
		{Type: "Cron", CronExpression: "* * *"},
		{Type: "Daily", Time: "08:00", Timezone: "Not/AZone"},
	}
	for _, config := range bad {
		s, r := serviceFor()
		got := s.Create(CreateRequest{ConnectionEpoch: 7, ClientToken: "token", Name: "Name", Prompt: "Prompt", Schedule: config})
		if got.OK || got.Error == nil || got.Error.Code != "invalidInput" || len(r.calls) != 0 {
			t.Fatalf("invalid schedule sent: %+v result=%+v", config, got)
		}
	}
}

func TestCASPauseOnlyAndErrorMetadata(t *testing.T) {
	s, r := serviceFor(taskWire)
	got := s.Update(UpdateRequest{ConnectionEpoch: 7, TaskID: "task-1", ExpectedVersion: 3, Enabled: boolPtr(false)})
	if !got.OK {
		t.Fatalf("pause failed %+v", got)
	}
	if len(r.calls) != 1 || r.calls[0].method != "POST" || r.calls[0].path != "/api/v1/esa-cron-tasks/task-1" {
		t.Fatal(r.calls)
	}
	if string(r.calls[0].body) != `{"expectedVersion":3,"enabled":false}` {
		t.Fatalf("incorrect CAS body: %s", r.calls[0].body)
	}
	conflict := dto.Failure("ScheduledTaskVersionConflict", "schedule", "reload", 409)
	conflict.CurrentState = &dto.CurrentState{Status: "Running"}
	r.errors = []*dto.SafeError{conflict}
	got = s.Update(UpdateRequest{ConnectionEpoch: 7, TaskID: "task-1", ExpectedVersion: 3, Enabled: boolPtr(true)})
	if got.OK || got.Value != nil || got.Error.Code != conflict.Code || got.Error.HTTPStatus != 409 || got.Error.CurrentState.Status != "Running" || len(r.calls) != 2 {
		t.Fatalf("conflict must preserve state and stop: %+v", got)
	}
	s2, r2 := serviceFor()
	invalid := s2.Update(UpdateRequest{ConnectionEpoch: 7, TaskID: "task-1", ExpectedVersion: 0, Enabled: boolPtr(false)})
	if invalid.OK || len(r2.calls) != 0 {
		t.Fatal("missing version reached server")
	}
}

func TestListQueriesAndAuthority(t *testing.T) {
	s, r := serviceFor(`{"items":[],"overview":{"executionCount":91,"scope":{"basis":"CurrentRunStatus","period":"AllTime","taskSelection":"AllMatchingTasks","includesAllPages":true}},"total":47,"limit":2,"offset":8,"nextOffset":10,"hasMore":true}`)
	got := s.List(ListRequest{ConnectionEpoch: 7, Query: "a&b + 中", SortBy: "name", SortDirection: "Desc", IncludeDisabled: boolPtr(false), Limit: 2, Offset: 8})
	if !got.OK || got.Value.Total == nil || *got.Value.Total != 47 || got.Value.Overview == nil || *got.Value.Overview.ExecutionCount != 91 || !got.Value.CountAvailable || !*got.Value.HasMore {
		t.Fatalf("authority lost: %+v", got)
	}
	u, _ := url.Parse(r.calls[0].path)
	q := u.Query()
	if q.Get("query") != "a&b + 中" || q.Get("includeDisabled") != "false" || q.Get("limit") != "2" || q.Get("offset") != "8" || q.Get("sortBy") != "name" || q.Get("sortDirection") != "Desc" {
		t.Fatalf("query mismatch: %v", q)
	}
	s2, _ := serviceFor(`{"items":[],"limit":20,"offset":0}`)
	missing := s2.List(ListRequest{ConnectionEpoch: 7})
	if !missing.OK || missing.Value.CountAvailable || missing.Value.Total != nil || missing.Value.HasMore != nil {
		t.Fatalf("missing authority was fabricated: %+v", missing.Value)
	}
	s3, _ := serviceFor(`{"items":[],"overview":{"executionCount":0},"total":0,"limit":20,"offset":0,"hasMore":false}`)
	zero := s3.List(ListRequest{ConnectionEpoch: 7})
	if !zero.OK || !zero.Value.CountAvailable || *zero.Value.Overview.ExecutionCount != 0 {
		t.Fatal("authoritative zero should be available")
	}
}

func TestRunSameTokenRecoveryAndPagination(t *testing.T) {
	wire := `{"run":{"id":"run-1","taskId":"task-1","status":"Queued","triggerType":"Manual","sessionId":"session-1","invocationId":"inv-1","errorMessage":"secret"}}`
	s, r := serviceFor(wire, wire)
	r.errors = []*dto.SafeError{dto.Failure("outcomeUnknown", "transport", "verifyOnly", 0), nil}
	request := RunRequest{ConnectionEpoch: 7, TaskID: "task-1", ClientToken: "same-token"}
	unknown := s.Run(request)
	if unknown.OK || unknown.Value != nil || unknown.ClientToken != "same-token" || unknown.Error == nil || len(r.calls) != 1 {
		t.Fatalf("unknown outcome changed token: %+v", unknown)
	}
	got := s.Run(request)
	if !got.OK || got.Value.Run.Status != "Queued" || got.ClientToken != "same-token" || len(r.calls) != 2 {
		t.Fatal(got)
	}
	if string(r.calls[0].body) != `{"clientToken":"same-token","mode":"Force"}` || string(r.calls[0].body) != string(r.calls[1].body) {
		t.Fatalf("recovery request changed: %+v", r.calls)
	}
	if r.calls[0].path != "/api/v1/esa-cron-tasks/task-1/run" {
		t.Fatal(r.calls)
	}
	s2, r2 := serviceFor(`{"items":[{"id":"run-1","taskId":"task-1","status":"Stuck","triggerType":"Scheduled"}],"total":50,"overview":{"executionCount":50},"limit":1,"offset":7,"nextOffset":8,"hasMore":true}`)
	history := s2.Runs(RunsRequest{ConnectionEpoch: 7, TaskID: "task-1", Limit: 1, Offset: 7})
	if !history.OK || history.Value.Items[0].Status != "Stuck" || *history.Value.Total != 50 || !history.Value.CountAvailable {
		t.Fatalf("run history mismatch: %+v", history)
	}
	if r2.calls[0].path != "/api/v1/esa-cron-tasks/task-1/runs?limit=1&offset=7" {
		t.Fatal(r2.calls)
	}
}

func TestReadUsesExactIDFromAuthorizedList(t *testing.T) {
	s, r := serviceFor(`{"items":[{"id":"other"}],"total":2,"limit":200,"offset":0,"nextOffset":1,"hasMore":true}`, `{"items":[{"id":"task-1","version":4}],"total":2,"limit":200,"offset":1,"hasMore":false}`)
	got := s.Read(TaskRequest{ConnectionEpoch: 7, TaskID: "task-1"})
	if !got.OK || got.Value.Task.ID != "task-1" || len(r.calls) != 2 {
		t.Fatalf("detail read failed %+v calls=%+v", got, r.calls)
	}
	for _, c := range r.calls {
		if !strings.HasPrefix(c.path, "/api/v1/esa-cron-tasks?") || c.method != "GET" {
			t.Fatalf("invented GET detail route: %+v", c)
		}
	}
	s2, _ := serviceFor(`{"items":[],"total":0,"limit":200,"offset":0,"hasMore":false}`)
	missing := s2.Read(TaskRequest{ConnectionEpoch: 7, TaskID: "task-1"})
	if missing.OK || missing.Error == nil || missing.Error.Code != "ScheduledTaskNotFound" {
		t.Fatal(missing)
	}
	s3, r3 := serviceFor(`{"items":[],"hasMore":true,"nextOffset":0}`)
	broken := s3.Read(TaskRequest{ConnectionEpoch: 7, TaskID: "task-1"})
	if broken.OK || broken.Error.Code != "invalidResponse" || len(r3.calls) != 1 {
		t.Fatal("nonadvancing pagination must stop")
	}
}

func TestDeleteDefinitionAndDeliveryTargets(t *testing.T) {
	s, r := serviceFor(`{"removed":true}`, `{"items":[{"channel":"Feishu","targetId":"oc_1","name":"Group","chatMode":"group"}]}`)
	deleted := s.Delete(TaskRequest{ConnectionEpoch: 7, TaskID: "task-1"})
	if !deleted.OK || !deleted.Value.Removed || deleted.Value.TaskID != "task-1" || len(r.calls) != 1 || r.calls[0].method != "DELETE" {
		t.Fatal(deleted)
	}
	encoded, _ := json.Marshal(deleted)
	if strings.Contains(string(encoded), "cancelled") {
		t.Fatal("definition deletion claimed cancellation")
	}
	targets := s.DeliveryTargets(DeliveryTargetsRequest{ConnectionEpoch: 7, Channel: "Feishu", AppID: "app + 1"})
	if !targets.OK || len(targets.Value.Items) != 1 {
		t.Fatal(targets)
	}
	u, _ := url.Parse(r.calls[1].path)
	if u.Path != "/api/v1/esa-cron-tasks/delivery-targets" || u.Query().Get("appId") != "app + 1" || u.Query().Get("channel") != "Feishu" {
		t.Fatal(r.calls)
	}
}

func TestIdentityAndStaleConnectionFailures(t *testing.T) {
	for _, status := range []int{401, 403, 422} {
		s, r := serviceFor()
		r.errors = []*dto.SafeError{dto.Failure("denied", "schedule", "never", status)}
		got := s.List(ListRequest{ConnectionEpoch: 7})
		if got.OK || got.Value != nil || got.Error.HTTPStatus != status || len(r.calls) != 1 || r.calls[0].family != "schedule" {
			t.Fatalf("identity failure swallowed: %+v", got)
		}
	}
	s, r := serviceFor()
	r.current = false
	got := s.List(ListRequest{ConnectionEpoch: 7})
	if got.OK || got.Error.Code != "staleConnection" || len(r.calls) != 0 {
		t.Fatal("stale connection reached server")
	}
	for _, id := range []string{"../secret", "task/run", "task?x=y", "task#fragment", "task%2Fescape", ""} {
		s, r := serviceFor()
		got := s.Delete(TaskRequest{ConnectionEpoch: 7, TaskID: id})
		if got.OK || len(r.calls) != 0 {
			t.Fatalf("unsafe ID reached server %q", id)
		}
	}
}

func TestUnknownCreatePreservesTokenAndNeverRetries(t *testing.T) {
	s, r := serviceFor()
	r.errors = []*dto.SafeError{dto.Failure("SchedulerUnavailable", "schedule", "readOnly", 503)}
	got := s.Create(CreateRequest{ConnectionEpoch: 7, ClientToken: "recover-create", Name: "Name", Prompt: "Prompt", Schedule: Config{Type: "Interval", IntervalSeconds: intPtr(60)}})
	if got.OK || got.ClientToken != "recover-create" || got.Value != nil || got.Error.HTTPStatus != 503 || got.Error.RetryDisposition == "readOnly" || len(r.calls) != 1 {
		t.Fatalf("uncertain write lost recovery: %+v", got)
	}
}

func TestExplicitUnavailableAuthorityWinsOverCounts(t *testing.T) {
	s, _ := serviceFor(`{"items":[],"countAvailable":false,"total":1,"overview":{"executionCount":91},"limit":20,"offset":0,"hasMore":false}`)
	got := s.List(ListRequest{ConnectionEpoch: 7})
	if !got.OK || got.Value.CountAvailable {
		t.Fatal("explicit unavailable authority was overwritten")
	}
	s2, _ := serviceFor(`{"items":[],"countAvailable":false,"total":1,"overview":{"executionCount":91},"limit":20,"offset":0,"hasMore":false}`)
	runs := s2.Runs(RunsRequest{ConnectionEpoch: 7, TaskID: "task-1"})
	if !runs.OK || runs.Value.CountAvailable {
		t.Fatal("run unavailable authority was overwritten")
	}
}

func TestDeleteRejectsMissingReceiptAndUpdateUnknownDoesNotRetry(t *testing.T) {
	s, r := serviceFor(`{}`)
	got := s.Delete(TaskRequest{ConnectionEpoch: 7, TaskID: "task-1"})
	if got.OK || got.Error == nil || got.Error.Code != "invalidResponse" || len(r.calls) != 1 {
		t.Fatalf("missing delete authority accepted: %+v", got)
	}
	s2, r2 := serviceFor()
	r2.errors = []*dto.SafeError{dto.Failure("outcomeUnknown", "transport", "verifyOnly", 0)}
	unknown := s2.Update(UpdateRequest{ConnectionEpoch: 7, TaskID: "task-1", ExpectedVersion: 3, Enabled: boolPtr(false)})
	if unknown.OK || unknown.Error == nil || unknown.Error.RetryDisposition != "verifyOnly" || len(r2.calls) != 1 {
		t.Fatal("CAS uncertain write must be read back before retry")
	}
}

func TestDeliveryAndBoundedInputValidation(t *testing.T) {
	badDeliveries := []Delivery{
		{Channel: "Web", TargetID: "oc_1"},
		{Channel: "Feishu", ReceiveIDType: "chat_id"},
		{Channel: "Feishu", TargetID: "oc_1", ThreadID: "omt_1"},
		{Channel: "Feishu", TargetID: "oc_1", ReceiveIDType: "open_id", ThreadID: "omt_1", ReplyMessageID: "om_1"},
		{Channel: "Unknown", TargetID: "target"},
	}
	for _, delivery := range badDeliveries {
		s, r := serviceFor()
		got := s.Create(CreateRequest{ConnectionEpoch: 7, ClientToken: "token", Name: "Name", Prompt: "Prompt", Schedule: Config{Type: "Daily", Time: "08:00"}, Delivery: &delivery})
		if got.OK || got.Error == nil || got.Error.Code != "invalidInput" || len(r.calls) != 0 {
			t.Fatalf("invalid delivery sent: %+v", delivery)
		}
	}
	for _, req := range []ListRequest{{ConnectionEpoch: 7, Limit: 201}, {ConnectionEpoch: 7, Offset: -1}, {ConnectionEpoch: 7, SortBy: "taskId"}, {ConnectionEpoch: 7, Query: strings.Repeat("中", 257)}} {
		s, r := serviceFor()
		got := s.List(req)
		if got.OK || len(r.calls) != 0 {
			t.Fatal("invalid paging/query reached server")
		}
	}
	for _, req := range []RunRequest{{ConnectionEpoch: 7, TaskID: "task-1", ClientToken: "token", Mode: "Resume"}, {ConnectionEpoch: 7, TaskID: "task-1", ClientToken: strings.Repeat("a", 129)}} {
		s, r := serviceFor()
		got := s.Run(req)
		if got.OK || len(r.calls) != 0 {
			t.Fatal("unsupported run mode/token reached server")
		}
	}
}

func TestReadPagesShareOneRequestDeadline(t *testing.T) {
	s, r := serviceFor(`{"items":[{"id":"other"}],"hasMore":true,"nextOffset":1}`, `{"items":[{"id":"task-1"}],"hasMore":false}`)
	got := s.Read(TaskRequest{ConnectionEpoch: 7, TaskID: "task-1"})
	if !got.OK || len(r.deadlines) != 2 || !r.deadlines[0].Equal(r.deadlines[1]) {
		t.Fatal("detail pagination reset its timeout on every page")
	}
}
