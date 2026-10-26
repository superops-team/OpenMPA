// Package schedule is the typed desktop adapter for server-owned ESA schedules.
package schedule

import "github.com/superops-team/OpenMPA/desktop/internal/dto"

// Config is a discriminated schedule. Only fields belonging to Type may be set.
// The server, rather than a desktop timer, interprets and executes this schedule.
type Config struct {
	Type            string `json:"type"`
	Timezone        string `json:"timezone,omitempty"`
	RunAt           string `json:"runAt,omitempty"`
	IntervalSeconds *int   `json:"intervalSeconds,omitempty"`
	AnchorAt        string `json:"anchorAt,omitempty"`
	Time            string `json:"time,omitempty"`
	Weekdays        []int  `json:"weekdays,omitempty"`
	MonthDays       []int  `json:"monthDays,omitempty"`
	CronExpression  string `json:"cronExpression,omitempty"`
}

type Delivery struct {
	AppID          string `json:"appId,omitempty"`
	Channel        string `json:"channel"`
	TargetID       string `json:"targetId,omitempty"`
	ReceiveIDType  string `json:"receiveIdType"`
	BestEffort     bool   `json:"bestEffort"`
	ThreadID       string `json:"threadId,omitempty"`
	ReplyMessageID string `json:"replyMessageId,omitempty"`
}

type Task struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	AgentID           string   `json:"agentId"`
	Prompt            string   `json:"prompt"`
	Enabled           bool     `json:"enabled"`
	Schedule          Config   `json:"schedule"`
	Delivery          Delivery `json:"delivery"`
	JitterSeconds     int      `json:"jitterSeconds"`
	TimeoutSeconds    int      `json:"timeoutSeconds"`
	NextRunAt         *string  `json:"nextRunAt"`
	RunningAt         *string  `json:"runningAt"`
	LastRunAt         *string  `json:"lastRunAt"`
	LastRunStatus     *string  `json:"lastRunStatus"`
	LastDurationMS    *int64   `json:"lastDurationMs"`
	ConsecutiveErrors int      `json:"consecutiveErrors"`
	Version           int      `json:"version"`
	CreatedAt         string   `json:"createdAt"`
	UpdatedAt         string   `json:"updatedAt"`
}

// Raw server status is preserved. Arbitrary lastError/errorMessage strings are
// intentionally excluded from IPC; they can contain upstream credentials or URLs.
type TaskRun struct {
	ID           string  `json:"id"`
	TaskID       string  `json:"taskId"`
	AgentID      string  `json:"agentId"`
	Status       string  `json:"status"`
	TriggerType  string  `json:"triggerType"`
	ScheduledAt  string  `json:"scheduledAt"`
	StartedAt    *string `json:"startedAt"`
	FinishedAt   *string `json:"finishedAt"`
	DurationMS   *int64  `json:"durationMs"`
	Model        *string `json:"model"`
	SessionID    *string `json:"sessionId"`
	InvocationID *string `json:"invocationId"`
	ErrorCode    *string `json:"errorCode"`
}

type StatusCounts struct {
	Queued    int64 `json:"Queued"`
	Running   int64 `json:"Running"`
	Stuck     int64 `json:"Stuck"`
	Succeeded int64 `json:"Succeeded"`
	Failed    int64 `json:"Failed"`
	TimedOut  int64 `json:"TimedOut"`
	Cancelled int64 `json:"Cancelled"`
	Skipped   int64 `json:"Skipped"`
}
type OverviewScope struct {
	Basis            string   `json:"basis"`
	Period           string   `json:"period"`
	TaskSelection    string   `json:"taskSelection"`
	IncludesAllPages bool     `json:"includesAllPages"`
	TriggerTypes     []string `json:"triggerTypes"`
}
type Overview struct {
	ExecutionCount    *int64         `json:"executionCount,omitempty"`
	SuccessCount      *int64         `json:"successCount,omitempty"`
	FailedCount       *int64         `json:"failedCount,omitempty"`
	SuccessRate       *float64       `json:"successRate,omitempty"`
	StatusCounts      *StatusCounts  `json:"statusCounts,omitempty"`
	Scope             *OverviewScope `json:"scope,omitempty"`
	AverageDurationMS *int64         `json:"averageDurationMs,omitempty"`
}
type TaskPage struct {
	Items          []Task    `json:"items"`
	Overview       *Overview `json:"overview,omitempty"`
	Total          *int64    `json:"total,omitempty"`
	Limit          int       `json:"limit"`
	Offset         int       `json:"offset"`
	NextOffset     *int      `json:"nextOffset,omitempty"`
	HasMore        *bool     `json:"hasMore,omitempty"`
	CountAvailable bool      `json:"countAvailable"`
}
type RunPage struct {
	Items          []TaskRun `json:"items"`
	Overview       *Overview `json:"overview,omitempty"`
	Total          *int64    `json:"total,omitempty"`
	Limit          int       `json:"limit"`
	Offset         int       `json:"offset"`
	NextOffset     *int      `json:"nextOffset,omitempty"`
	HasMore        *bool     `json:"hasMore,omitempty"`
	CountAvailable bool      `json:"countAvailable"`
}
type TaskValue struct {
	Task Task `json:"task"`
}
type RunValue struct {
	Run TaskRun `json:"run"`
}
type DeleteValue struct {
	TaskID  string `json:"taskId"`
	Removed bool   `json:"removed"`
}
type DeliveryTarget struct {
	AppID    *string `json:"appId,omitempty"`
	Channel  string  `json:"channel"`
	TargetID string  `json:"targetId"`
	Name     string  `json:"name"`
	ChatMode string  `json:"chatMode"`
	Avatar   *string `json:"avatar"`
}
type DeliveryTargetPage struct {
	Items []DeliveryTarget `json:"items"`
}

type TaskRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	TaskID          string `json:"taskId"`
}
type ListRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	IncludeDisabled *bool  `json:"includeDisabled,omitempty"`
	Query           string `json:"query,omitempty"`
	SortBy          string `json:"sortBy,omitempty"`
	SortDirection   string `json:"sortDirection,omitempty"`
	Limit           int    `json:"limit,omitempty"`
	Offset          int    `json:"offset,omitempty"`
}
type RunsRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	TaskID          string `json:"taskId"`
	Limit           int    `json:"limit,omitempty"`
	Offset          int    `json:"offset,omitempty"`
}
type CreateRequest struct {
	ConnectionEpoch uint64    `json:"connectionEpoch"`
	ClientToken     string    `json:"clientToken"`
	Name            string    `json:"name"`
	AgentID         string    `json:"agentId,omitempty"`
	Prompt          string    `json:"prompt"`
	Enabled         *bool     `json:"enabled,omitempty"`
	Schedule        Config    `json:"schedule"`
	Delivery        *Delivery `json:"delivery,omitempty"`
	JitterSeconds   *int      `json:"jitterSeconds,omitempty"`
	TimeoutSeconds  *int      `json:"timeoutSeconds,omitempty"`
}
type UpdateRequest struct {
	ConnectionEpoch uint64    `json:"connectionEpoch"`
	TaskID          string    `json:"taskId"`
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
type RunRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	TaskID          string `json:"taskId"`
	ClientToken     string `json:"clientToken"`
	Mode            string `json:"mode,omitempty"`
}
type DeliveryTargetsRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	Channel         string `json:"channel,omitempty"`
	AppID           string `json:"appId,omitempty"`
}

type ListResult struct {
	OK    bool           `json:"ok"`
	Value *TaskPage      `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}
type TaskResult struct {
	OK    bool           `json:"ok"`
	Value *TaskValue     `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}
type CreateResult struct {
	OK          bool           `json:"ok"`
	Value       *TaskValue     `json:"value,omitempty"`
	Error       *dto.SafeError `json:"error,omitempty"`
	ClientToken string         `json:"clientToken"`
}
type RunResult struct {
	OK          bool           `json:"ok"`
	Value       *RunValue      `json:"value,omitempty"`
	Error       *dto.SafeError `json:"error,omitempty"`
	ClientToken string         `json:"clientToken"`
}
type RunsResult struct {
	OK    bool           `json:"ok"`
	Value *RunPage       `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}
type DeleteResult struct {
	OK    bool           `json:"ok"`
	Value *DeleteValue   `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}
type DeliveryTargetsResult struct {
	OK    bool                `json:"ok"`
	Value *DeliveryTargetPage `json:"value,omitempty"`
	Error *dto.SafeError      `json:"error,omitempty"`
}
