package nativeui

import (
	"context"
	"fmt"
	"github.com/egoist/mygo/ui"
	"github.com/google/uuid"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"github.com/superops-team/OpenMPA/desktop/internal/knowledge"
)

type importIntent struct {
	Text    *knowledge.TextImportRequest `json:"text,omitempty"`
	File    *knowledge.FileImportRequest `json:"file,omitempty"`
	Version bool                         `json:"version"`
}
type knowledgeState struct {
	sessionID, name, text, query string
	items                        []knowledge.Item
	selected                     *knowledge.Item
	receipt                      *knowledge.ImportReceipt
	pending                      *importIntent
	content                      *knowledge.ContentPage
	results                      *knowledge.SearchPage
}

func (p *Controller) knowledgeMember() bool {
	for _, s := range p.currentSessions() {
		if s.ID == p.knowledge.sessionID {
			return true
		}
	}
	return false
}
func (p *Controller) loadKnowledge() {
	r := knowledge.SessionRequest{ConnectionEpoch: p.epoch(), SessionID: p.knowledge.sessionID}
	if !p.knowledgeMember() {
		p.err = dto.Failure("invalidInput", "mpa", "never", 0)
		return
	}
	scope := p.services.Connection.Status().Value.Scope + "/knowledge/" + r.SessionID
	p.async(func(ctx context.Context) (func(), *dto.SafeError) {
		intent, intentError := p.intents.load(scope)
		out := p.services.Knowledge.List(ctx, r)
		return func() {
			p.knowledge.pending = intent.Import
			if intentError != nil {
				p.err = dto.Failure("pendingPersistenceFailed", "desktop", "never", 0)
			}
			if out.OK && out.Value != nil {
				p.knowledge.items = out.Value.Items
			}
		}, out.Error
	})
}
func (p *Controller) renderKnowledge(c *ui.Context) {
	s := &p.knowledge
	if !p.services.Connection.Status().Value.Connected {
		p.note(c, "Connect a Runtime and select a Session to manage knowledge.")
		return
	}
	p.section(c, "Session knowledge", func() {
		options := []string{}
		choice := ""
		for _, v := range p.currentSessions() {
			label := v.Title + " (" + v.ID + ")"
			options = append(options, label)
			if v.ID == s.sessionID {
				choice = label
			}
		}
		ui.Select(c.Key("Knowledge Session"), &choice, options).Label("Knowledge Session").Disabled(p.busy || s.pending != nil).OnChange(func() {
			for _, v := range p.currentSessions() {
				if choice == v.Title+" ("+v.ID+")" {
					p.knowledge = knowledgeState{sessionID: v.ID}
					p.loadKnowledge()
					return
				}
			}
		})

		if !p.knowledgeMember() {
			p.note(c, "Choose a Session above.")
			return
		}
		p.note(c, "Session "+s.sessionID)
		p.button(c, "Refresh knowledge", p.loadKnowledge)
		for _, item := range s.items {
			item := item
			ui.Row(c.Key(fmt.Sprintf("%s:%d", item.KnowledgeID, item.Version))).Gap(10).Children(func() {
				ui.Column(c).Grow(1).Children(func() {
					ui.Text(c, item.DisplayName).FontSize(14)
					p.note(c, fmt.Sprintf("Version %d · %s · %s", item.Version, item.SourceType, item.Status))
				})
				p.button(c, "Open knowledge "+item.KnowledgeID, func() { s.selected = &item; s.content = nil; s.results = nil })
			})
		}
	})
	if !p.knowledgeMember() {
		return
	}
	p.section(c, "Import knowledge", func() {
		p.field(c, "Knowledge name", &s.name, false)
		p.multiline(c, "Knowledge text", &s.text)
		ui.Row(c).Gap(8).Children(func() {
			p.button(c, "Import text", func() { p.startKnowledgeImport(false, false) })
			p.button(c, "Import file", func() { p.startKnowledgeImport(true, false) })
			if s.selected != nil {
				p.button(c, "Add text version", func() { p.startKnowledgeImport(false, true) })
				p.button(c, "Add file version", func() { p.startKnowledgeImport(true, true) })
			}
		})
		p.note(c, "File imports use the native file picker. URL import is unavailable in the existing API.")
		if s.pending != nil {
			pending := *s.pending
			key := ""
			if pending.Text != nil {
				key = pending.Text.IdempotencyKey
			}
			if pending.File != nil {
				key = pending.File.IdempotencyKey
			}
			p.note(c, "Import "+key+" requires verification. Keep the original content or file.")
			p.button(c, "Recover original knowledge import", func() { p.ask("Recover knowledge import", key, func() { p.submitKnowledgeImport(pending) }) })
		}
		if s.receipt != nil {
			r := *s.receipt
			p.note(c, fmt.Sprintf("Import %s · Knowledge %s · Version %d · %s", r.ImportID, r.KnowledgeID, r.Version, r.Status))
			p.button(c, "Read import status", func() {
				request := knowledge.ImportStatusRequest{ConnectionEpoch: p.epoch(), SessionID: s.sessionID, ImportID: r.ImportID}
				p.async(func(ctx context.Context) (func(), *dto.SafeError) {
					out := p.services.Knowledge.ImportStatus(ctx, request)
					return func() {
						if out.OK && out.Value != nil {
							p.knowledge.selected = out.Value
							p.message = out.Value.Status
						}
					}, out.Error
				})
			})
		}
	})
	if s.selected != nil {
		item := *s.selected
		r := knowledge.ItemRequest{ConnectionEpoch: p.epoch(), SessionID: s.sessionID, KnowledgeID: item.KnowledgeID, Version: &item.Version}
		p.section(c, "Knowledge · "+item.DisplayName, func() {
			p.note(c, fmt.Sprintf("%s · version %d · %s", item.KnowledgeID, item.Version, item.Status))
			p.button(c, "Read knowledge details", func() {
				p.async(func(ctx context.Context) (func(), *dto.SafeError) {
					out := p.services.Knowledge.Read(ctx, r)
					return func() {
						if out.OK {
							p.knowledge.selected = out.Value
						}
					}, out.Error
				})
			})
			p.button(c, "Read knowledge content", func() {
				request := knowledge.ContentRequest{ConnectionEpoch: r.ConnectionEpoch, SessionID: r.SessionID, KnowledgeID: r.KnowledgeID, Version: r.Version, Limit: 4096}
				p.readKnowledgeContent(request)
			})
			if s.content != nil {
				page := s.content
				ui.Text(c, page.Content).Selectable()
				p.note(c, "Citation: "+page.Citation.Ref+" · "+page.Completeness)
				if page.NextCursor != nil {
					cursor := *page.NextCursor
					p.button(c, "Next content page", func() {
						request := knowledge.ContentRequest{ConnectionEpoch: r.ConnectionEpoch, SessionID: r.SessionID, KnowledgeID: r.KnowledgeID, Version: r.Version, Cursor: &cursor, Limit: 4096}
						p.readKnowledgeContent(request)
					})
				}
			}
			p.button(c, "Delete knowledge", func() {
				request := r
				request.Version = nil
				p.ask("Delete knowledge", item.DisplayName+" · "+item.KnowledgeID, func() {
					p.async(func(ctx context.Context) (func(), *dto.SafeError) {
						out := p.services.Knowledge.Delete(ctx, request)
						return func() {
							if out.OK {
								p.knowledge.selected = nil
								p.knowledge.content = nil
								p.loadKnowledge()
							}
						}, out.Error
					})
				})
			})
		})
	}
	p.section(c, "Search knowledge", func() {
		p.field(c, "Knowledge search query", &s.query, false)
		p.button(c, "Search session knowledge", func() {
			r := knowledge.SearchRequest{ConnectionEpoch: p.epoch(), SessionID: s.sessionID, Query: s.query, Limit: 10}
			p.searchKnowledge(r)
		})
		if s.results != nil {
			page := s.results
			for _, item := range page.Items {
				item := item
				ui.Column(c.Key(item.Ref)).Gap(4).Children(func() {
					ui.Text(c, item.DisplayName).FontSize(14)
					ui.Text(c, item.Content).Selectable()
					p.note(c, fmt.Sprintf("Citation %s · version %d", item.Citation.Ref, item.Citation.Version))
				})
			}
			p.note(c, "Search: "+page.Completeness)
			if page.NextCursor != nil {
				cursor := *page.NextCursor
				p.button(c, "Next search page", func() {
					r := knowledge.SearchRequest{ConnectionEpoch: p.epoch(), SessionID: s.sessionID, Query: s.query, Cursor: &cursor, Limit: 10}
					p.searchKnowledge(r)
				})
			}
		}
	})
}
func (p *Controller) readKnowledgeContent(r knowledge.ContentRequest) {
	p.async(func(ctx context.Context) (func(), *dto.SafeError) {
		out := p.services.Knowledge.Content(ctx, r)
		return func() {
			if out.OK {
				p.knowledge.content = out.Value
			}
		}, out.Error
	})
}
func (p *Controller) searchKnowledge(r knowledge.SearchRequest) {
	p.async(func(ctx context.Context) (func(), *dto.SafeError) {
		out := p.services.Knowledge.Search(ctx, r)
		return func() {
			if out.OK {
				p.knowledge.results = out.Value
			}
		}, out.Error
	})
}
func (p *Controller) startKnowledgeImport(file, version bool) {
	if p.knowledge.pending != nil {
		p.message = "Recover the original import before starting another submission."
		return
	}
	s := &p.knowledge
	id := ""
	if version {
		if s.selected == nil {
			return
		}
		id = s.selected.KnowledgeID
	}
	input := importIntent{Version: version}
	key := uuid.NewString()
	if file {
		input.File = &knowledge.FileImportRequest{ConnectionEpoch: p.epoch(), SessionID: s.sessionID, KnowledgeID: id, IdempotencyKey: key, DisplayName: s.name}
	} else {
		input.Text = &knowledge.TextImportRequest{ConnectionEpoch: p.epoch(), SessionID: s.sessionID, KnowledgeID: id, IdempotencyKey: key, DisplayName: s.name, Content: s.text}
	}
	p.submitKnowledgeImport(input)
}
func (p *Controller) submitKnowledgeImport(input importIntent) {
	s := &p.knowledge
	scope := p.services.Connection.Status().Value.Scope + "/knowledge/" + s.sessionID
	epoch := p.epoch()
	if input.Text != nil {
		v := *input.Text
		v.ConnectionEpoch = epoch
		input.Text = &v
	}
	if input.File != nil {
		v := *input.File
		v.ConnectionEpoch = epoch
		input.File = &v
	}
	s.pending = &input
	p.async(func(ctx context.Context) (func(), *dto.SafeError) {
		if e := p.intents.saveImport(scope, &input); e != nil {
			return nil, dto.Failure("pendingPersistenceFailed", "desktop", "never", 0)
		}
		var out knowledge.ImportResult
		if input.Text != nil {
			if input.Version {
				out = p.services.Knowledge.AddTextVersion(ctx, *input.Text)
			} else {
				out = p.services.Knowledge.ImportText(ctx, *input.Text)
			}
		} else if input.File != nil {
			out = knowledge.ImportFileFromWindow(p.services.Knowledge, ctx, p.window, *input.File, input.Version)
		} else {
			return nil, dto.Failure("invalidInput", "desktop", "never", 0)
		}
		clear := out.OK || (out.Error != nil && (out.Error.RetryDisposition == "never" || out.Error.Code == "cancelled"))
		if clear {
			if e := p.intents.saveImport(scope, nil); e != nil {
				return nil, dto.Failure("pendingPersistenceFailed", "desktop", "never", 0)
			}
		}
		return func() {
			if clear {
				p.knowledge.pending = nil
			}
			if out.Value != nil {
				p.knowledge.receipt = out.Value
				p.loadKnowledge()
			}
		}, out.Error
	})
}
