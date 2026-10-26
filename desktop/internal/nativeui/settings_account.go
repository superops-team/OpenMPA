package nativeui

import (
	"context"

	"github.com/egoist/mygo/ui"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"github.com/superops-team/OpenMPA/desktop/internal/localization"
)

func (p *Controller) renderAccount(c *ui.Context) {
	s := &p.settings
	platform := p.services.Platform.Status().Value
	vaultFirst := s.vault.Exists && !platform.SignedIn
	if vaultFirst {
		p.renderVault(c, platform)
	}
	p.section(c, "Volcengine account", func() {
		if platform.SignedIn && platform.Principal != nil {
			p.note(c, "Account "+platform.Principal.AccountID+" · "+platform.Principal.IdentityType)
			p.button(c, "Sign out", func() {
				p.identityAsync(func(context.Context) (func(), *dto.SafeError) {
					r := p.services.Platform.SignOut()
					return nil, r.Error
				})
			})
		} else {
			p.field(c, "Access key", &s.ak, !s.showCredentials)
			p.field(c, "Secret key", &s.sk, !s.showCredentials)
			p.field(c, "Security token (optional)", &s.securityToken, !s.showCredentials)
			ui.Checkbox(c, &s.showCredentials, localization.Text("showCredentials", nil)).Disabled(p.busy)
			p.renderRegionSelect(c, "Login region")
			p.note(c, "Credentials remain in this OpenMPA process and are cleared on exit.")
			p.button(c, "Sign in", func() {
				r := dto.SignInRequest{AccessKey: s.ak, SecretKey: s.sk, SecurityToken: s.securityToken, Region: s.region}
				p.identityAsync(func(ctx context.Context) (func(), *dto.SafeError) {
					out := p.services.Platform.SignIn(ctx, r)
					return func() {
						if out.OK {
							p.settings.ak = ""
							p.settings.sk = ""
							p.settings.securityToken = ""
							p.settings.tab = "Runtime"
							p.loadDirectory(true)
						}
					}, out.Error
				})
			})
		}
	})
	if !vaultFirst {
		p.renderVault(c, platform)
	}
}

func (p *Controller) refreshVault() {
	if p.services.Platform == nil {
		return
	}
	result := p.services.Platform.VaultStatus()
	if result.OK && result.Value != nil {
		p.settings.vault = *result.Value
	}
}

func (p *Controller) renderVault(c *ui.Context, platform *dto.PlatformState) {
	s := &p.settings
	p.section(c, "Saved credentials", func() {
		if s.vault.Exists {
			p.note(c, localization.Text("vaultAccount", map[string]any{"account": s.vault.AccountID, "region": s.vault.Region}))
			if platform.SignedIn {
				p.note(c, localization.Text("vaultEnabled", nil))
			} else {
				p.note(c, localization.Text("vaultUnlockHint", nil))
				p.field(c, "Passphrase", &s.vaultPassphrase, !s.showVault)
				ui.Checkbox(c, &s.showVault, localization.Text("showPassphrase", nil)).Disabled(p.busy)
				p.button(c, "Unlock and sign in", func() {
					passphrase := s.vaultPassphrase
					p.identityAsync(func(ctx context.Context) (func(), *dto.SafeError) {
						out := p.services.Platform.UnlockVault(ctx, dto.VaultPassphraseRequest{Passphrase: passphrase})
						return func() {
							if out.OK {
								p.settings.vaultPassphrase = ""
								p.settings.vaultConfirm = ""
								p.settings.tab = "Runtime"
								p.refreshVault()
								p.loadDirectory(true)
							}
						}, out.Error
					})
				})
			}
			p.button(c, "Forget saved credentials", func() {
				p.ask("Forget saved credentials", "Delete the encrypted credential from this Mac? You will need to enter AK/SK again.", func() {
					p.async(func(context.Context) (func(), *dto.SafeError) {
						r := p.services.Platform.ForgetVault()
						return func() {
							if r.OK {
								p.settings.vaultPassphrase = ""
								p.settings.vaultConfirm = ""
								p.refreshVault()
								p.message = "Saved credentials removed."
							}
						}, r.Error
					})
				})
			})
			return
		}
		if !platform.SignedIn {
			p.note(c, localization.Text("vaultSignInFirst", nil))
			return
		}
		p.note(c, localization.Text("vaultHint", nil))
		p.field(c, "Passphrase", &s.vaultPassphrase, !s.showVault)
		p.field(c, "Confirm passphrase", &s.vaultConfirm, !s.showVault)
		ui.Checkbox(c, &s.showVault, localization.Text("showPassphrase", nil)).Disabled(p.busy)
		p.button(c, "Save credentials", func() {
			passphrase, confirm := s.vaultPassphrase, s.vaultConfirm
			if passphrase != confirm {
				p.err = dto.Failure("invalidInput", "desktop", "never", 0)
				return
			}
			p.async(func(ctx context.Context) (func(), *dto.SafeError) {
				r := p.services.Platform.SaveVault(ctx, dto.VaultPassphraseRequest{Passphrase: passphrase})
				return func() {
					if r.OK {
						p.settings.vaultPassphrase = ""
						p.settings.vaultConfirm = ""
						p.refreshVault()
						p.message = "Credentials saved locally with your passphrase."
					}
				}, r.Error
			})
		})
	})
}
