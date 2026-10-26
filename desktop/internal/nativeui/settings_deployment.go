package nativeui

import (
	"context"
	"fmt"
	"strconv"

	"github.com/egoist/mygo/ui"
	"github.com/google/uuid"
	"github.com/superops-team/OpenMPA/desktop/internal/deployment"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"github.com/superops-team/OpenMPA/desktop/internal/localization"
	"github.com/superops-team/OpenMPA/desktop/internal/runtimeconfig"
)

func (p *Controller) renderDeployment(c *ui.Context) {
	s := &p.settings
	if !p.services.Connection.Status().Value.Connected {
		p.note(c, "Choose and connect a Runtime first.")
		return
	}
	p.section(c, "Runtime resources", func() {
		p.button(c, "Read current resources", func() {
			r := runtimeconfig.ScopeRequest{ConnectionEpoch: p.epoch()}
			p.async(func(context.Context) (func(), *dto.SafeError) {
				out := p.services.RuntimeConfig.Read(r)
				return func() {
					if out.OK && out.Value != nil {
						v := out.Value
						p.settings.config = v
						p.settings.description = v.Values.Description
						p.settings.cpu = strconv.Itoa(v.Values.CPUMilli)
						p.settings.memory = strconv.Itoa(v.Values.MemoryMB)
						p.settings.min = strconv.Itoa(v.Values.MinInstance)
						p.settings.max = strconv.Itoa(v.Values.MaxInstance)
						p.settings.concurrency = strconv.Itoa(v.Values.MaxConcurrency)
					}
				}, out.Error
			})
		})
		if s.config != nil {
			p.note(c, fmt.Sprintf("%s · version %d · %s", s.config.RuntimeID, s.config.Version, s.config.Status))
			p.field(c, "Description", &s.description, false)
			ui.Row(c).Gap(14).Children(func() {
				ui.Column(c).Grow(1).Children(func() { p.field(c, "CPU (milli cores)", &s.cpu, false) })
				ui.Column(c).Grow(1).Children(func() { p.field(c, "Memory (MB)", &s.memory, false) })
			})
			ui.Row(c).Gap(14).Children(func() {
				ui.Column(c).Grow(1).Children(func() { p.field(c, "Minimum instances", &s.min, false) })
				ui.Column(c).Grow(1).Children(func() { p.field(c, "Maximum instances", &s.max, false) })
				ui.Column(c).Grow(1).Children(func() { p.field(c, "Maximum concurrency", &s.concurrency, false) })
			})
			p.button(c, "Update Runtime resources", func() {
				v, e := s.configValues()
				if e != nil {
					p.err = e
					return
				}
				r := runtimeconfig.UpdateRequest{ConnectionEpoch: p.epoch(), OperationID: uuid.NewString(), Fingerprint: s.config.Fingerprint, Values: v, Confirm: true}
				p.ask("Update Runtime resources", fmt.Sprintf("%s · CPU %d · Memory %d MB · Instances %d–%d · Concurrency %d", s.config.RuntimeID, v.CPUMilli, v.MemoryMB, v.MinInstance, v.MaxInstance, v.MaxConcurrency), func() {
					p.settings.configOperation = &runtimeconfig.Operation{OperationID: r.OperationID, RuntimeID: p.services.Connection.Status().Value.RuntimeID, Target: r.Values, State: "submitted"}
					p.async(func(context.Context) (func(), *dto.SafeError) {
						out := p.services.RuntimeConfig.Update(r)
						return func() {
							p.settings.config = nil
							if out.Value != nil {
								p.settings.configOperation = out.Value
							}
						}, out.Error
					})
				})
			})
		}
		p.field(c, "Resource update operation ID", &s.resourceReceiptID, false)
		p.button(c, "Recover resource update receipt", func() {
			r := runtimeconfig.ReadRequest{ScopeRequest: runtimeconfig.ScopeRequest{ConnectionEpoch: p.epoch()}, OperationID: s.resourceReceiptID}
			p.async(func(context.Context) (func(), *dto.SafeError) {
				out := p.services.RuntimeConfig.ReadUpdate(r)
				return func() {
					if out.Value != nil {
						p.settings.configOperation = out.Value
					}
				}, out.Error
			})
		})
		if s.configOperation != nil {
			op := *s.configOperation
			p.note(c, op.OperationID+" · "+op.State)
			p.button(c, "Read resource update receipt", func() {
				r := runtimeconfig.ReadRequest{ScopeRequest: runtimeconfig.ScopeRequest{ConnectionEpoch: p.epoch()}, OperationID: op.OperationID}
				p.async(func(context.Context) (func(), *dto.SafeError) {
					out := p.services.RuntimeConfig.ReadUpdate(r)
					return func() {
						if out.Value != nil {
							p.settings.configOperation = out.Value
						}
					}, out.Error
				})
			})
		}
	})
	p.section(c, "Runtime and Worker Tool images", func() {
		p.button(c, "Read current images", func() {
			r := deployment.ScopeRequest{ConnectionEpoch: p.epoch()}
			p.async(func(context.Context) (func(), *dto.SafeError) {
				out := p.services.Deployment.ReadImages(r)
				return func() {
					if out.OK && out.Value != nil {
						p.settings.images = out.Value
						p.settings.runtimeImage = out.Value.RuntimeImage
						p.settings.workerImage = out.Value.WorkerImage
					}
				}, out.Error
			})
		})
		if s.images != nil {
			p.note(c, "Runtime "+s.images.RuntimeID+" · version "+s.images.RuntimeVersion+" · "+s.images.RuntimeStatus)
			p.note(c, "Worker Tool "+s.images.ToolID+" · "+s.images.WorkerStatus)
			p.field(c, "Runtime image", &s.runtimeImage, false)
			p.button(c, "Update Runtime image", func() { p.updateImage(false) })
			p.field(c, "Worker image", &s.workerImage, false)
			p.button(c, "Update Worker Tool image", func() { p.updateImage(true) })
		}
		p.button(c, "Refresh saved image receipts", func() {
			r := deployment.ScopeRequest{ConnectionEpoch: p.epoch()}
			p.async(func(context.Context) (func(), *dto.SafeError) {
				out := p.services.Deployment.ListUpdates(r)
				return func() {
					if out.OK && out.Value != nil {
						p.settings.operations = out.Value.Operations
					}
				}, out.Error
			})
		})
		for _, op := range s.operations {
			op := op
			ui.Column(c.Key(op.OperationID)).Gap(5).Children(func() {
				p.note(c, op.Kind+" · "+op.OperationID+" · "+op.State)
				p.note(c, op.Message)
				p.buttonWithKey(c, "read-receipt:"+op.OperationID, localization.Text("readReceipt", map[string]any{"operation": op.OperationID}), func() {
					r := deployment.ReadUpdateRequest{ConnectionEpoch: p.epoch(), OperationID: op.OperationID}
					p.async(func(context.Context) (func(), *dto.SafeError) {
						out := p.services.Deployment.ReadUpdate(r)
						return func() {
							if out.Value != nil {
								p.rememberImageOperation(*out.Value)
							}
						}, out.Error
					})
				})
			})
		}
	})
}
func (p *Controller) rememberImageOperation(op deployment.Operation) {
	for i, v := range p.settings.operations {
		if v.OperationID == op.OperationID {
			p.settings.operations[i] = op
			return
		}
	}
	p.settings.operations = append(p.settings.operations, op)
}
func (p *Controller) updateImage(worker bool) {
	s := &p.settings
	if s.images == nil {
		return
	}
	images := *s.images
	epoch := p.epoch()
	opID := uuid.NewString()
	target := s.runtimeImage
	kind := "Runtime"
	resource := images.RuntimeID
	if worker {
		target = s.workerImage
		kind = "Worker Tool"
		resource = images.ToolID
	}
	p.ask("Update "+kind+" image", resource+"\n"+target, func() {
		p.rememberImageOperation(deployment.Operation{OperationID: opID, Kind: kind, RuntimeID: images.RuntimeID, ToolID: images.ToolID, TargetImage: target, State: "submitted"})
		p.async(func(context.Context) (func(), *dto.SafeError) {
			var out deployment.OperationResult
			if worker {
				out = p.services.Deployment.UpdateWorkerImage(deployment.WorkerImageRequest{ConnectionEpoch: epoch, OperationID: opID, TargetImage: target, ReviewFingerprint: images.ReviewFingerprint})
			} else {
				out = p.services.Deployment.UpdateRuntimeImage(deployment.RuntimeImageRequest{ConnectionEpoch: epoch, OperationID: opID, TargetImage: target, ReviewFingerprint: images.ReviewFingerprint, ReleaseEnable: true})
			}
			return func() {
				p.settings.images = nil
				if out.Value != nil {
					p.rememberImageOperation(*out.Value)
				}
			}, out.Error
		})
	})
}
