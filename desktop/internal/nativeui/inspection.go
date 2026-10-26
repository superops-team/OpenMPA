package nativeui

import "context"

// Inspection is a read-only acceptance projection, collected on the UI thread.
// It exposes rendered domain identity and counts, never credentials or text drafts.
// Inspect is a Go helper and is not a bound IPC method.
type Inspection struct {
	CreatedSessionID                                                     string
	Busy                                                                 bool
	TaskDraft                                                            bool
	View, RuntimeID, ResourceRuntimeID, ToolID, TaskID, RunID, RunStatus string
	RuntimeVersion                                                       int
	CPUMilli                                                             int
	ChannelIDs                                                           []string
	CredentialsEmpty                                                     bool
}

func Inspect(ctx context.Context, p *Controller) (Inspection, bool) {
	out := make(chan Inspection, 1)
	p.window.Update(func() {
		v := Inspection{CreatedSessionID: p.createdSessionID, Busy: p.busy, TaskDraft: p.schedules.draft != nil, View: p.shell.View, RuntimeID: p.services.Connection.Status().Value.RuntimeID, CredentialsEmpty: p.settings.ak == "" && p.settings.sk == "" && p.settings.jwt == "" && p.settings.channelKey == "" && p.channels.secret == ""}
		if p.settings.config != nil {
			v.ResourceRuntimeID = p.settings.config.RuntimeID
			v.RuntimeVersion = p.settings.config.Version
			v.CPUMilli = p.settings.config.Values.CPUMilli
		}
		if p.settings.images != nil {
			v.ToolID = p.settings.images.ToolID
		}
		for _, a := range p.channels.accounts {
			v.ChannelIDs = append(v.ChannelIDs, a.Channel+":"+a.AppID)
		}
		if p.schedules.detail != nil {
			v.TaskID = p.schedules.detail.ID
		}
		if p.schedules.accepted != nil {
			v.RunID = p.schedules.accepted.ID
			v.RunStatus = p.schedules.accepted.Status
		}
		out <- v
	})
	select {
	case v := <-out:
		return v, true
	case <-ctx.Done():
		return Inspection{}, false
	}
}
