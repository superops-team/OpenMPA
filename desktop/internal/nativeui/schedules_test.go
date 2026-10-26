package nativeui

import (
	"github.com/superops-team/OpenMPA/desktop/internal/schedule"
	"testing"
)

func TestNativeScheduleOnlySendsFieldsForSelectedType(t *testing.T) {
	d := taskDraft{kind: "Weekly", timezone: "Asia/Shanghai", at: "09:30", days: "1,3,5", runAt: "ignored", cron: "ignored", interval: "ignored", jitter: "0", timeout: "300"}
	v, e := d.config()
	if e != nil || v.Type != "Weekly" || len(v.Weekdays) != 3 || v.RunAt != "" || v.CronExpression != "" || v.IntervalSeconds != nil {
		t.Fatal(v, e)
	}
	d.days = "1,nope"
	if _, e = d.config(); e == nil {
		t.Fatal("invalid days accepted")
	}
	d.kind = "Once"
	d.runAt = "2026-10-26 14:35"
	v, e = d.config()
	if e != nil || v.RunAt != "2026-10-26T14:35:00+08:00" || v.Time != "" || v.Weekdays != nil {
		t.Fatal(v, e)
	}
}
func TestNativeScheduleEditPreservesServerDeliveryAndVersion(t *testing.T) {
	task := schedule.Task{ID: "t", Version: 8, Name: "Report", Schedule: schedule.Config{Type: "Once", Timezone: "UTC", RunAt: "2026-10-26T03:00:00Z"}, Delivery: schedule.Delivery{Channel: "Feishu", AppID: "bot", TargetID: "chat", ReceiveIDType: "chat_id", ThreadID: "omt_test", ReplyMessageID: "om_test"}, TimeoutSeconds: 300}
	d := draftFromTask(task)
	if d.original == nil || d.original.Version != 8 || d.threadID != "omt_test" || d.replyID != "om_test" || d.runAt != "2026-10-26 03:00" {
		t.Fatal("server fields lost")
	}
}

func TestNativeTaskNameEditKeepsExactOnceTimestamp(t *testing.T) {
	original := schedule.Task{ID: "task", Name: "Old", Schedule: schedule.Config{Type: "Once", Timezone: "America/New_York", RunAt: "2026-11-01T01:30:43.123456789-05:00"}}
	d := draftFromTask(original)
	d.name = "New"
	c, e := d.config()
	if e != nil || c.RunAt != original.Schedule.RunAt {
		t.Fatal("once precision or DST offset lost", c, e)
	}
}
