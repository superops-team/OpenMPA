package nativeui

import (
	"github.com/egoist/mygo/ui"
	"github.com/superops-team/OpenMPA/desktop/internal/channels"
	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"testing"
)

func TestChannelBindingFieldsRemainProviderSpecific(t *testing.T) {
	s := channelState{channel: "feishu", mode: "manual", appID: "app", secret: "private", domain: "lark", bindingType: "shared"}
	f := s.bindingRequest(7, "op")
	if f.AppID != "app" || f.AppSecret != "private" || f.ClientID != "" || f.Secret != "" {
		t.Fatal("feishu request mixed providers")
	}
	s.channel = "dingtalk"
	d := s.bindingRequest(7, "op")
	if d.ClientID != "app" || d.ClientSecret != "private" || d.AppID != "" || d.Domain != "" {
		t.Fatal("dingtalk request mixed providers")
	}
	s.channel = "wecom"
	w := s.bindingRequest(7, "op")
	if w.BotID != "app" || w.Secret != "private" || w.ClientID != "" || w.BindingType != "" {
		t.Fatal("wecom request mixed providers")
	}
}
func TestChannelSelectionUsesProviderAndAccount(t *testing.T) {
	s := channelState{selected: "feishu:shared-id", accounts: []channels.Account{{Channel: "dingtalk", AppID: "shared-id"}, {Channel: "feishu", AppID: "shared-id"}}}
	a, ok := s.account()
	if !ok || a.Channel != "feishu" {
		t.Fatal("account dimension lost")
	}
	s.selected = "wecom:shared-id"
	if _, ok = s.account(); ok {
		t.Fatal("unknown account accepted")
	}
}

func TestNativeChannelCatalogueRowRemainsReachable(t *testing.T) {
	p := testController(t)
	_, e := p.services.Connection.Client.Bind(connection.Binding{Endpoint: "https://example.com", Scope: "test", RuntimeID: "r", RuntimeKey: "key"}, false)
	if e != nil {
		t.Fatal(e)
	}
	p.channels = newChannelState()
	p.channels.loaded = true
	p.channels.accounts = []channels.Account{{Channel: "feishu", AppID: "account-id", AppName: "Test"}}
	p.channels.capabilities = &channels.Capabilities{ManagementScope: "runtimeAdmin"}
	tt := ui.NewTester(p.renderChannels, 760, 700)
	r, ok := tt.Find("Manage feishu:account-id")
	if !ok {
		t.Fatal("virtual channel row missing")
	}
	if r.H <= 0 {
		t.Fatal(r)
	}
	if e := tt.Click("Manage feishu:account-id"); e != nil {
		t.Fatal(e)
	}
	if p.channels.selected != "feishu:account-id" {
		t.Fatal("account was not selected")
	}
}
