package nativeui

import (
	"context"
	"fmt"
	"github.com/egoist/mygo/ui"
	"github.com/google/uuid"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"github.com/superops-team/OpenMPA/desktop/internal/schedule"
	"strconv"
	"strings"
	"time"
)

type ScheduleService interface {
	List(schedule.ListRequest) schedule.ListResult
	Read(schedule.TaskRequest) schedule.TaskResult
	Create(schedule.CreateRequest) schedule.CreateResult
	Update(schedule.UpdateRequest) schedule.TaskResult
	Run(schedule.RunRequest) schedule.RunResult
	Runs(schedule.RunsRequest) schedule.RunsResult
	Delete(schedule.TaskRequest) schedule.DeleteResult
	DeliveryTargets(schedule.DeliveryTargetsRequest) schedule.DeliveryTargetsResult
}
type taskDraft struct {
	original                                                                                      *schedule.Task
	name, prompt, agent, kind, timezone, runAt, interval, anchor, at, days, cron, jitter, timeout string
	enabled                                                                                       bool
	deliveryChannel, appID, targetID, receiveIDType, threadID, replyID                            string
}
type scheduleState struct {
	query         string
	page          *schedule.TaskPage
	detail        *schedule.Task
	runs          *schedule.RunPage
	draft         *taskDraft
	targets       []schedule.DeliveryTarget
	pendingCreate *schedule.CreateRequest
	pendingRun    *schedule.RunRequest
	accepted      *schedule.TaskRun
}

func defaultTaskDraft() taskDraft {
	return taskDraft{kind: "Once", timezone: "Asia/Shanghai", runAt: func() string {
		loc, _ := time.LoadLocation("Asia/Shanghai")
		return time.Now().Add(time.Hour).In(loc).Format("2006-01-02 15:04")
	}(), jitter: "0", timeout: "300", enabled: true, deliveryChannel: "Web", receiveIDType: "chat_id"}
}
func draftFromTask(t schedule.Task) taskDraft {
	d := taskDraft{original: &t, name: t.Name, prompt: t.Prompt, agent: t.AgentID, kind: t.Schedule.Type, timezone: t.Schedule.Timezone, runAt: t.Schedule.RunAt, anchor: t.Schedule.AnchorAt, at: t.Schedule.Time, cron: t.Schedule.CronExpression, jitter: strconv.Itoa(t.JitterSeconds), timeout: strconv.Itoa(t.TimeoutSeconds), enabled: t.Enabled, deliveryChannel: t.Delivery.Channel, appID: t.Delivery.AppID, targetID: t.Delivery.TargetID, receiveIDType: t.Delivery.ReceiveIDType, threadID: t.Delivery.ThreadID, replyID: t.Delivery.ReplyMessageID}
	if d.timezone == "" {
		d.timezone = "Asia/Shanghai"
	}
	if d.deliveryChannel == "" {
		d.deliveryChannel = "Web"
	}
	if d.receiveIDType == "" {
		d.receiveIDType = "chat_id"
	}
	if value, e := time.Parse(time.RFC3339Nano, d.runAt); e == nil {
		if loc, e := time.LoadLocation(d.timezone); e == nil {
			format := "2006-01-02 15:04"
			if value.Second() != 0 || value.Nanosecond() != 0 {
				format = "2006-01-02 15:04:05.999999999"
			}
			d.runAt = value.In(loc).Format(format)
		}
	}
	if t.Schedule.IntervalSeconds != nil {
		d.interval = strconv.Itoa(*t.Schedule.IntervalSeconds)
	}
	days := t.Schedule.Weekdays
	if d.kind == "Monthly" {
		days = t.Schedule.MonthDays
	}
	var words []string
	for _, v := range days {
		words = append(words, strconv.Itoa(v))
	}
	d.days = strings.Join(words, ",")
	return d
}
func (d *taskDraft) config() (schedule.Config, *dto.SafeError) {
	invalid := func() (schedule.Config, *dto.SafeError) {
		return schedule.Config{}, dto.Failure("invalidInput", "mpa", "never", 0)
	}
	loc, e := time.LoadLocation(d.timezone)
	if e != nil || d.timezone == "Local" {
		return invalid()
	}
	c := schedule.Config{Type: d.kind, Timezone: d.timezone}
	switch d.kind {
	case "Once":
		if d.original != nil && d.original.Schedule.Type == "Once" && d.timezone == d.original.Schedule.Timezone && d.runAt == draftFromTask(*d.original).runAt {
			c.RunAt = d.original.Schedule.RunAt
			break
		}
		var value time.Time
		valid := false
		for _, format := range []string{"2006-01-02 15:04", "2006-01-02 15:04:05.999999999", time.RFC3339Nano} {
			parsed, err := time.ParseInLocation(format, d.runAt, loc)
			if err == nil {
				if format != time.RFC3339Nano {
					wall := strings.Split(d.runAt, ".")[0]
					base := "2006-01-02 15:04"
					if len(wall) > 16 {
						base = "2006-01-02 15:04:05"
					}
					if parsed.Format(base) != wall {
						continue
					}
				}
				value = parsed
				valid = true
				break
			}
		}
		if !valid {
			return invalid()
		}
		c.RunAt = value.Format(time.RFC3339Nano)

	case "Interval":
		n, e := strconv.Atoi(d.interval)
		if e != nil || n < 30 || n > 31536000 {
			return invalid()
		}
		c.IntervalSeconds = &n
		c.AnchorAt = d.anchor
	case "Daily", "Weekly", "Monthly":
		if _, e := time.Parse("15:04", d.at); e != nil {
			return invalid()
		}
		c.Time = d.at
		if d.kind != "Daily" {
			max := 7
			if d.kind == "Monthly" {
				max = 31
			}
			seen := map[int]bool{}
			days := []int{}
			for _, v := range strings.Split(d.days, ",") {
				n, e := strconv.Atoi(strings.TrimSpace(v))
				if e != nil || n < 1 || n > max || seen[n] {
					return invalid()
				}
				seen[n] = true
				days = append(days, n)
			}
			if d.kind == "Weekly" {
				c.Weekdays = days
			} else {
				c.MonthDays = days
			}
		}
	case "Cron":
		if !required(d.cron) {
			return invalid()
		}
		c.CronExpression = d.cron
	default:
		return invalid()
	}
	return c, nil
}
func (d *taskDraft) delivery() schedule.Delivery {
	v := schedule.Delivery{Channel: d.deliveryChannel, ReceiveIDType: d.receiveIDType, BestEffort: true}
	if d.deliveryChannel == "Feishu" {
		v.AppID = d.appID
		v.TargetID = d.targetID
		v.ThreadID = d.threadID
		v.ReplyMessageID = d.replyID
	}
	return v
}
func (d *taskDraft) seconds() (int, int, *dto.SafeError) {
	j, e := strconv.Atoi(d.jitter)
	t, e2 := strconv.Atoi(d.timeout)
	if e != nil || e2 != nil || j < 0 || j > 86400 || t < 30 || t > 86400 {
		return 0, 0, dto.Failure("invalidInput", "mpa", "never", 0)
	}
	return j, t, nil
}
func (p *Controller) loadTasks(offset int) {
	all := true
	scope := p.services.Connection.Status().Value.Scope
	r := schedule.ListRequest{ConnectionEpoch: p.epoch(), Query: p.schedules.query, IncludeDisabled: &all, Limit: 50, Offset: offset}
	p.async(func(context.Context) (func(), *dto.SafeError) {
		intent, intentError := p.intents.load(scope)
		out := p.services.Schedule.List(r)
		return func() {
			if out.OK {
				p.schedules.page = out.Value
				p.schedules.pendingCreate = intent.Create
				p.schedules.pendingRun = intent.Run
				if intentError != nil {
					p.err = dto.Failure("pendingPersistenceFailed", "desktop", "never", 0)
				}
			}
		}, out.Error
	})
}
func (p *Controller) readTask(id string) {
	r := schedule.TaskRequest{ConnectionEpoch: p.epoch(), TaskID: id}
	p.async(func(context.Context) (func(), *dto.SafeError) {
		out := p.services.Schedule.Read(r)
		return func() {
			if out.OK && out.Value != nil {
				if p.schedules.detail == nil || p.schedules.detail.ID != out.Value.Task.ID {
					p.schedules.accepted = nil
				}
				p.schedules.detail = &out.Value.Task
				p.schedules.runs = nil
			}
		}, out.Error
	})
}
func (p *Controller) renderSchedules(c *ui.Context) {
	s := &p.schedules
	if !p.services.Connection.Status().Value.Connected {
		p.note(c, "Connect a Runtime to manage scheduled tasks.")
		return
	}
	p.note(c, "Tasks run on the Runtime and continue when OpenMPA is closed.")
	ui.Row(c).Gap(8).Children(func() {
		p.button(c, "New scheduled task", func() {
			if s.pendingCreate != nil {
				p.message = "Recover the existing submission before creating another task."
				return
			}
			v := defaultTaskDraft()
			s.draft = &v
		})
		p.button(c, "Refresh scheduled tasks", func() { p.loadTasks(0) })
	})
	p.field(c, "Find scheduled tasks", &s.query, false)
	p.button(c, "Search scheduled tasks", func() { p.loadTasks(0) })
	if s.pendingCreate != nil {
		pending := *s.pendingCreate
		p.note(c, "Submission "+pending.ClientToken+" requires verification; the original form is retained.")
		p.button(c, "Recover original task submission", func() {
			p.ask("Recover scheduled task", pending.Name+" · "+pending.ClientToken, func() { pending.ConnectionEpoch = p.epoch(); p.submitTaskCreate(pending) })
		})
	}
	if s.pendingRun != nil {
		pending := *s.pendingRun
		p.note(c, "Trigger "+pending.ClientToken+" requires verification.")
		p.button(c, "Recover original task trigger", func() {
			p.ask("Recover task trigger", pending.TaskID+" · "+pending.ClientToken, func() { pending.ConnectionEpoch = p.epoch(); p.submitTaskRun(pending) })
		})
	}
	if s.draft != nil {
		p.renderTaskDraft(c)
		return
	}
	if s.detail != nil {
		p.button(c, "Back to scheduled tasks", func() { s.detail = nil; s.runs = nil; s.accepted = nil })
		p.renderTaskDetail(c)
		return
	}
	if s.page != nil {
		page := s.page
		p.section(c, "Scheduled tasks", func() {
			if page.CountAvailable && page.Total != nil {
				p.note(c, fmt.Sprintf("%d tasks", *page.Total))
			} else {
				p.note(c, "Authoritative task count is unavailable.")
			}
			p.rows(c, "task-directory", len(page.Items), 230, func(i int) {
				t := page.Items[i]

				ui.Row(c.Key(t.ID)).Gap(12).Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Children(func() {
						ui.Text(c, t.Name).FontSize(14).SingleLine()
						p.note(c, fmt.Sprintf("%s · Enabled: %t · version %d", t.Schedule.Type, t.Enabled, t.Version))
					})
					p.button(c, "Open task "+t.ID, func() { p.readTask(t.ID) })
				})
			})
			ui.Row(c).Gap(8).Children(func() {
				if page.Offset > 0 {
					p.button(c, "Previous tasks", func() { p.loadTasks(max(0, page.Offset-page.Limit)) })
				}
				if page.NextOffset != nil {
					next := *page.NextOffset
					p.button(c, "Next tasks", func() { p.loadTasks(next) })
				}
			})
		})
	}

}
func (p *Controller) renderTaskDraft(c *ui.Context) {
	d := p.schedules.draft
	p.section(c, "Scheduled task editor", func() {
		p.field(c, "Task name", &d.name, false)
		p.multiline(c, "Task instructions", &d.prompt)
		p.field(c, "Agent ID (optional)", &d.agent, false)
		ui.Checkbox(c, &d.enabled, "Task enabled").Disabled(p.busy)
		ui.Select(c.Key("Schedule type"), &d.kind, []string{"Once", "Interval", "Daily", "Weekly", "Monthly", "Cron"}).Label("Schedule type").Disabled(p.busy)
		p.field(c, "Timezone", &d.timezone, false)
		switch d.kind {
		case "Once":
			p.field(c, "Run at (YYYY-MM-DD HH:MM)", &d.runAt, false)
		case "Interval":
			p.field(c, "Interval (seconds)", &d.interval, false)
			p.field(c, "Anchor time (optional RFC3339)", &d.anchor, false)
		case "Daily", "Weekly", "Monthly":
			p.field(c, "Time (HH:MM)", &d.at, false)
			if d.kind == "Weekly" {
				p.field(c, "Weekdays (1=Mon through 7=Sun)", &d.days, false)
			}
			if d.kind == "Monthly" {
				p.field(c, "Month days (1–31)", &d.days, false)
			}
		case "Cron":
			p.field(c, "Cron expression", &d.cron, false)
		}
		ui.Row(c).Gap(14).Children(func() {
			ui.Column(c).Grow(1).Children(func() { p.field(c, "Jitter (seconds)", &d.jitter, false) })
			ui.Column(c).Grow(1).Children(func() { p.field(c, "Timeout (seconds)", &d.timeout, false) })
		})
		ui.Select(c.Key("Delivery channel"), &d.deliveryChannel, []string{"Web", "Feishu"}).Label("Delivery channel").Disabled(p.busy)
		if d.deliveryChannel == "Feishu" {
			p.field(c, "Delivery application ID", &d.appID, false)
			ui.Select(c.Key("Recipient type"), &d.receiveIDType, []string{"chat_id", "open_id"}).Label("Recipient type").Disabled(p.busy)
			p.field(c, "Delivery recipient ID", &d.targetID, false)
			p.field(c, "Thread ID (optional)", &d.threadID, false)
			p.field(c, "Reply message ID (optional)", &d.replyID, false)
			p.button(c, "Read delivery targets", func() {
				r := schedule.DeliveryTargetsRequest{ConnectionEpoch: p.epoch(), Channel: "Feishu", AppID: d.appID}
				p.async(func(context.Context) (func(), *dto.SafeError) {
					out := p.services.Schedule.DeliveryTargets(r)
					return func() {
						if out.OK && out.Value != nil {
							p.schedules.targets = out.Value.Items
						}
					}, out.Error
				})
			})
			for _, v := range p.schedules.targets {
				v := v
				p.button(c, "Use recipient "+v.TargetID, func() {
					d.targetID = v.TargetID
					d.receiveIDType = "chat_id"
					if v.AppID != nil {
						d.appID = *v.AppID
					}
				})
			}
		}
		p.button(c, "Save scheduled task", func() {
			config, e := d.config()
			if e != nil {
				p.err = e
				return
			}
			j, t, e := d.seconds()
			if e != nil {
				p.err = e
				return
			}
			delivery := d.delivery()
			if d.original == nil {
				request := schedule.CreateRequest{ConnectionEpoch: p.epoch(), ClientToken: uuid.NewString(), Name: d.name, Prompt: d.prompt, AgentID: d.agent, Enabled: &d.enabled, Schedule: config, Delivery: &delivery, JitterSeconds: &j, TimeoutSeconds: &t}
				enabled := d.enabled
				request.Enabled = &enabled
				p.ask("Create scheduled task", d.name+" · "+d.kind+" · "+d.timezone, func() { p.submitTaskCreate(request) })
			} else {
				name, prompt, enabled, agent := d.name, d.prompt, d.enabled, d.agent
				request := schedule.UpdateRequest{ConnectionEpoch: p.epoch(), TaskID: d.original.ID, ExpectedVersion: d.original.Version, Name: &name, Prompt: &prompt, Enabled: &enabled, Schedule: &config, Delivery: &delivery, JitterSeconds: &j, TimeoutSeconds: &t}
				if agent != "" {
					request.AgentID = &agent
				}
				p.ask("Update scheduled task", d.original.ID+fmt.Sprintf(" · version %d", d.original.Version), func() {
					p.async(func(context.Context) (func(), *dto.SafeError) {
						out := p.services.Schedule.Update(request)
						return func() {
							if out.OK && out.Value != nil {
								p.schedules.detail = &out.Value.Task
								p.schedules.draft = nil
								p.loadTasks(0)
							}
						}, out.Error
					})
				})
			}
		})
		p.button(c, "Cancel task editing", func() { p.schedules.draft = nil })
	})
}
func (p *Controller) submitTaskCreate(r schedule.CreateRequest) {
	scope := p.services.Connection.Status().Value.Scope
	p.schedules.pendingCreate = &r
	p.async(func(context.Context) (func(), *dto.SafeError) {
		if e := p.intents.change(scope, "create", &r, nil); e != nil {
			return nil, dto.Failure("pendingPersistenceFailed", "desktop", "never", 0)
		}
		out := p.services.Schedule.Create(r)
		if out.OK || (out.Error != nil && out.Error.RetryDisposition == "never") {
			if e := p.intents.change(scope, "create", nil, nil); e != nil {
				return nil, dto.Failure("pendingPersistenceFailed", "desktop", "never", 0)
			}
		}
		return func() {
			if out.OK && out.Value != nil {
				p.schedules.pendingCreate = nil
				p.schedules.detail = &out.Value.Task
				p.schedules.draft = nil
				p.message = "Scheduled task created: " + out.Value.Task.ID
				p.loadTasks(0)
			} else if out.Error != nil && out.Error.RetryDisposition == "never" {
				p.schedules.pendingCreate = nil
			}
		}, out.Error
	})
}
func (p *Controller) submitTaskRun(r schedule.RunRequest) {
	scope := p.services.Connection.Status().Value.Scope
	p.schedules.pendingRun = &r
	p.async(func(context.Context) (func(), *dto.SafeError) {
		if e := p.intents.change(scope, "run", nil, &r); e != nil {
			return nil, dto.Failure("pendingPersistenceFailed", "desktop", "never", 0)
		}
		out := p.services.Schedule.Run(r)
		if out.OK || (out.Error != nil && out.Error.RetryDisposition == "never") {
			if e := p.intents.change(scope, "run", nil, nil); e != nil {
				return nil, dto.Failure("pendingPersistenceFailed", "desktop", "never", 0)
			}
		}
		return func() {
			if out.OK && out.Value != nil {
				p.schedules.pendingRun = nil
				p.schedules.accepted = &out.Value.Run
				p.message = "Run accepted: " + out.Value.Run.ID
			} else if out.Error != nil && out.Error.RetryDisposition == "never" {
				p.schedules.pendingRun = nil
			}
		}, out.Error
	})
}

func (p *Controller) renderTaskDetail(c *ui.Context) {
	s := &p.schedules
	task := *s.detail
	r := schedule.TaskRequest{ConnectionEpoch: p.epoch(), TaskID: task.ID}
	p.section(c, "Task · "+task.Name, func() {
		p.note(c, task.ID+fmt.Sprintf(" · version %d · Enabled: %t", task.Version, task.Enabled))
		ui.Text(c, task.Prompt).Selectable()
		if task.NextRunAt != nil {
			p.note(c, "Next run: "+*task.NextRunAt)
		}
		ui.Row(c).Gap(8).Children(func() {
			p.button(c, "Edit task", func() { d := draftFromTask(task); s.draft = &d })
			p.button(c, "Reload task", func() { p.readTask(task.ID) })
			label := "Enable task"
			if task.Enabled {
				label = "Pause task"
			}
			p.button(c, label, func() {
				v := !task.Enabled
				request := schedule.UpdateRequest{ConnectionEpoch: r.ConnectionEpoch, TaskID: task.ID, ExpectedVersion: task.Version, Enabled: &v}
				p.async(func(context.Context) (func(), *dto.SafeError) {
					out := p.services.Schedule.Update(request)
					return func() {
						if out.OK && out.Value != nil {
							p.schedules.detail = &out.Value.Task
							p.loadTasks(0)
						}
					}, out.Error
				})
			})
			p.button(c, "Run task now", func() {
				if s.pendingRun != nil {
					p.message = "Recover the original trigger before starting another run."
					return
				}
				request := schedule.RunRequest{ConnectionEpoch: r.ConnectionEpoch, TaskID: task.ID, ClientToken: uuid.NewString(), Mode: "Force"}
				p.ask("Run scheduled task now", task.Name+" · "+task.ID, func() { p.submitTaskRun(request) })
			})
		})
		p.button(c, "Delete scheduled task", func() {
			p.ask("Delete scheduled task", task.Name+" · "+task.ID, func() {
				p.async(func(context.Context) (func(), *dto.SafeError) {
					out := p.services.Schedule.Delete(r)
					return func() {
						if out.OK && out.Value != nil && out.Value.Removed {
							p.schedules.detail = nil
							p.schedules.draft = nil
							p.loadTasks(0)
						}
					}, out.Error
				})
			})
		})
		p.button(c, "Read task run history", func() { p.loadTaskRuns(task.ID, 0) })
		if s.accepted != nil {
			p.renderTaskRun(c, *s.accepted)
		}
		if s.runs != nil {
			page := s.runs
			if page.CountAvailable && page.Total != nil {
				p.note(c, fmt.Sprintf("%d executions", *page.Total))
			} else {
				p.note(c, "Authoritative execution count is unavailable; no estimate is shown.")
			}
			for _, run := range page.Items {
				if s.accepted != nil && run.ID == s.accepted.ID {
					continue
				}
				p.renderTaskRun(c, run)
			}
			ui.Row(c).Gap(8).Children(func() {
				if page.Offset > 0 {
					p.button(c, "Previous runs", func() { p.loadTaskRuns(task.ID, max(0, page.Offset-page.Limit)) })
				}
				if page.NextOffset != nil {
					next := *page.NextOffset
					p.button(c, "Next runs", func() { p.loadTaskRuns(task.ID, next) })
				}
			})
		}
	})
}
func (p *Controller) loadTaskRuns(task string, offset int) {
	r := schedule.RunsRequest{ConnectionEpoch: p.epoch(), TaskID: task, Limit: 50, Offset: offset}
	p.async(func(context.Context) (func(), *dto.SafeError) {
		out := p.services.Schedule.Runs(r)
		return func() {
			if out.OK {
				p.schedules.runs = out.Value
			}
		}, out.Error
	})
}
func (p *Controller) renderTaskRun(c *ui.Context, run schedule.TaskRun) {
	ui.Column(c.Key(run.ID)).Gap(4).Padding(10).Border(1, ui.RGB(230, 230, 230)).Radius(6).Children(func() {
		ui.Text(c, run.Status+" · "+run.ID).FontSize(13)
		p.note(c, run.TriggerType+" · "+run.ScheduledAt)
		if run.ErrorCode != nil {
			p.note(c, "Error: "+*run.ErrorCode)
		}
		if run.SessionID != nil {
			p.note(c, "Session: "+*run.SessionID)
		}
		if run.InvocationID != nil {
			p.note(c, "Invocation: "+*run.InvocationID)
		}
	})
}
