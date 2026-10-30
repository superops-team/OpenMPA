package schedule

import (
	"regexp"
	"sort"
	"strings"
	"time"
	_ "time/tzdata"
	"unicode/utf8"
)

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,256}$`)
var hhmmPattern = regexp.MustCompile(`^(?:[01][0-9]|2[0-3]):[0-5][0-9]$`)
var threadPattern = regexp.MustCompile(`^omt_[A-Za-z0-9]+$`)
var replyPattern = regexp.MustCompile(`^om_[A-Za-z0-9]+$`)
var errorCodePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)

func textValid(s string, max int) bool {
	return utf8.ValidString(s) && strings.TrimSpace(s) != "" && utf8.RuneCountInString(s) <= max
}
func pageValid(limit, offset int) bool { return limit >= 0 && limit <= 200 && offset >= 0 }
func timestampValid(s string) bool     { _, e := time.Parse(time.RFC3339Nano, s); return e == nil }
func daysValid(days []int, max int) bool {
	if len(days) < 1 || len(days) > max {
		return false
	}
	seen := make(map[int]bool, len(days))
	for _, d := range days {
		if d < 1 || d > max || seen[d] {
			return false
		}
		seen[d] = true
	}
	return true
}

func normalizeConfig(c Config) (Config, bool) {
	c.Timezone = strings.TrimSpace(c.Timezone)
	if c.Timezone == "" {
		c.Timezone = "Asia/Shanghai"
	}
	// LoadLocation("Local") depends on the host and is not an IANA schedule zone.
	if c.Timezone == "Local" {
		return c, false
	}
	if _, e := time.LoadLocation(c.Timezone); e != nil {
		return c, false
	}
	allowed := map[string]bool{}
	valid := false
	switch c.Type {
	case "Once":
		allowed["runAt"] = true
		valid = timestampValid(c.RunAt)
	case "Interval":
		allowed["intervalSeconds"] = true
		allowed["anchorAt"] = true
		valid = c.IntervalSeconds != nil && *c.IntervalSeconds >= 30 && *c.IntervalSeconds <= 31536000 && (c.AnchorAt == "" || timestampValid(c.AnchorAt))
	case "Daily":
		allowed["time"] = true
		valid = hhmmPattern.MatchString(c.Time)
	case "Weekly":
		allowed["time"] = true
		allowed["weekdays"] = true
		valid = hhmmPattern.MatchString(c.Time) && daysValid(c.Weekdays, 7)
		c.Weekdays = append([]int(nil), c.Weekdays...)
		sort.Ints(c.Weekdays)
	case "Monthly":
		allowed["time"] = true
		allowed["monthDays"] = true
		valid = hhmmPattern.MatchString(c.Time) && daysValid(c.MonthDays, 31)
		c.MonthDays = append([]int(nil), c.MonthDays...)
		sort.Ints(c.MonthDays)
	case "Cron":
		allowed["cronExpression"] = true
		c.CronExpression, valid = normalizeCron(c.CronExpression)
	}
	set := map[string]bool{"runAt": c.RunAt != "", "intervalSeconds": c.IntervalSeconds != nil, "anchorAt": c.AnchorAt != "", "time": c.Time != "", "weekdays": c.Weekdays != nil, "monthDays": c.MonthDays != nil, "cronExpression": c.CronExpression != ""}
	for key, present := range set {
		if present && !allowed[key] {
			return c, false
		}
	}
	return c, valid
}

// Match the server's minute-precision Quartz compatibility normalization. Final
// cron-field grammar is validated by the server's ScheduleConverter.
func normalizeCron(expression string) (string, bool) {
	if !textValid(expression, 128) {
		return expression, false
	}
	fields := strings.Fields(expression)
	hasNumber := func(s string) bool { return strings.ContainsAny(s, "0123456789") }
	if len(fields) == 7 && fields[0] == "0" && (fields[6] == "*" || fields[6] == "?") {
		if hasNumber(fields[5]) {
			return expression, false
		}
		fields = fields[1:6]
	}
	if len(fields) == 6 && fields[0] == "0" {
		if fields[5] == "*" || fields[5] == "?" {
			fields = fields[1:]
		} else if fields[3] == "?" || fields[5] == "?" {
			if hasNumber(fields[5]) {
				return expression, false
			}
			fields = fields[1:]
		} else {
			return expression, false
		}
	}
	if len(fields) != 5 {
		return expression, false
	}
	for i, field := range fields {
		if field == "?" {
			fields[i] = "*"
		}
	}
	return strings.Join(fields, " "), true
}

func normalizeDelivery(d Delivery) (Delivery, bool) {
	if d.Channel == "" {
		d.Channel = "Web"
	}
	if d.ReceiveIDType == "" {
		d.ReceiveIDType = "chat_id"
	}
	// Server delivery is always best effort. False cannot request stronger delivery.
	d.BestEffort = true
	if d.AppID != "" && !textValid(d.AppID, 256) {
		return d, false
	}
	if d.ThreadID != "" || d.ReplyMessageID != "" {
		if d.Channel != "Feishu" || d.ReceiveIDType != "chat_id" || !threadPattern.MatchString(d.ThreadID) || !replyPattern.MatchString(d.ReplyMessageID) || len(d.ThreadID) > 512 || len(d.ReplyMessageID) > 512 {
			return d, false
		}
	}
	switch d.Channel {
	case "Web":
		if d.AppID != "" || d.ReceiveIDType != "chat_id" || strings.TrimSpace(d.TargetID) != "" {
			return d, false
		}
		d.TargetID = ""
	case "Feishu":
		d.TargetID = strings.TrimSpace(d.TargetID)
		if !textValid(d.TargetID, 512) || (d.ReceiveIDType != "chat_id" && d.ReceiveIDType != "open_id") {
			return d, false
		}
	default:
		return d, false
	}
	return d, true
}

func secondsValid(jitter, timeout *int) bool {
	return (jitter == nil || (*jitter >= 0 && *jitter <= 86400)) && (timeout == nil || (*timeout >= 30 && *timeout <= 86400))
}
