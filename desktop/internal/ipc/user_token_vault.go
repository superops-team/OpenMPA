package ipc

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"github.com/superops-team/OpenMPA/desktop/internal/vault"
)

// UserTokenVaultStatus is a local read; it never returns idToken or refreshToken.
func (p *Platform) UserTokenVaultStatus(r dto.UserTokenVaultRequest) dto.UserTokenVaultStatusResult {
	if p.Vault == nil {
		return dto.UserTokenVaultStatusResult{OK: true, Value: &dto.UserTokenVaultStatus{}}
	}
	if !validUserTokenVaultRuntimeID(r.RuntimeID) {
		return dto.UserTokenVaultStatusResult{Error: dto.Failure("invalidInput", "desktop", "never", 0)}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	status, err := p.Vault.UserTokenStatus(ctx, r.RuntimeID)
	if err != nil {
		return dto.UserTokenVaultStatusResult{Error: dto.Failure("vaultUnavailable", "desktop", "readOnly", 0)}
	}
	out := userTokenVaultStatus(status)
	return dto.UserTokenVaultStatusResult{OK: true, Value: &out}
}

// SaveUserTokenVault stores Runtime user identity tokens without exposing them
// through later status or unlock responses.
func (p *Platform) SaveUserTokenVault(ctx context.Context, r dto.UserTokenVaultSaveRequest) dto.UserTokenVaultStatusResult {
	if p.Vault == nil {
		return dto.UserTokenVaultStatusResult{Error: dto.Failure("vaultUnavailable", "desktop", "never", 0)}
	}
	passphrase := []byte(r.Passphrase)
	token := vault.UserToken{
		RuntimeID:    r.RuntimeID,
		Issuer:       r.Issuer,
		ClientID:     r.ClientID,
		Subject:      r.Subject,
		MPAID:        r.MPAID,
		SpaceID:      r.SpaceID,
		IDToken:      r.IDToken,
		RefreshToken: r.RefreshToken,
		ExpiresAt:    r.ExpiresAt,
	}
	r.Passphrase, r.IDToken, r.RefreshToken = "", "", ""
	defer clear(passphrase)
	defer func() { token.IDToken, token.RefreshToken = "", "" }()
	if err := p.Vault.SaveUserToken(ctx, passphrase, token); err != nil {
		if errors.Is(err, vault.ErrInvalid) {
			return dto.UserTokenVaultStatusResult{Error: dto.Failure("invalidInput", "desktop", "never", 0)}
		}
		return dto.UserTokenVaultStatusResult{Error: dto.Failure("vaultUnavailable", "desktop", "readOnly", 0)}
	}
	return p.UserTokenVaultStatus(dto.UserTokenVaultRequest{RuntimeID: token.RuntimeID})
}

// UnlockUserTokenVault validates the passphrase and returns only safe metadata.
func (p *Platform) UnlockUserTokenVault(ctx context.Context, r dto.UserTokenVaultUnlockRequest) dto.UserTokenVaultStatusResult {
	if p.Vault == nil {
		return dto.UserTokenVaultStatusResult{Error: dto.Failure("vaultUnavailable", "desktop", "never", 0)}
	}
	if !validUserTokenVaultRuntimeID(r.RuntimeID) {
		return dto.UserTokenVaultStatusResult{Error: dto.Failure("invalidInput", "desktop", "never", 0)}
	}
	passphrase := []byte(r.Passphrase)
	r.Passphrase = ""
	defer clear(passphrase)
	token, err := p.Vault.UnlockUserToken(ctx, passphrase, r.RuntimeID)
	defer func() { token.IDToken, token.RefreshToken = "", "" }()
	if err != nil {
		switch {
		case errors.Is(err, vault.ErrMissing):
			return dto.UserTokenVaultStatusResult{Error: dto.Failure("vaultMissing", "desktop", "never", 0)}
		case errors.Is(err, vault.ErrWrongPassphrase):
			return dto.UserTokenVaultStatusResult{Error: dto.Failure("vaultWrongPassphrase", "desktop", "never", 0)}
		default:
			return dto.UserTokenVaultStatusResult{Error: dto.Failure("vaultUnavailable", "desktop", "readOnly", 0)}
		}
	}
	if r.ConnectionEpoch != 0 {
		state := p.Connection.State()
		if !state.Connected || state.ConnectionEpoch != r.ConnectionEpoch {
			return dto.UserTokenVaultStatusResult{Error: dto.Failure("staleConnection", "transport", "reload", 0)}
		}
		if state.RuntimeID != token.RuntimeID {
			return dto.UserTokenVaultStatusResult{Error: dto.Failure("invalidInput", "desktop", "never", 0)}
		}
		if err := p.Connection.SetBusinessAuth(dto.IdentityRequest{ConnectionEpoch: r.ConnectionEpoch, AuthMode: "legacyJwt", LegacyJWT: token.IDToken}); err != nil {
			return dto.UserTokenVaultStatusResult{Error: err}
		}
	}
	return p.UserTokenVaultStatus(dto.UserTokenVaultRequest{RuntimeID: token.RuntimeID})
}

func (p *Platform) ForgetUserTokenVault(r dto.UserTokenVaultRequest) dto.EmptyResult {
	if p.Vault == nil {
		return dto.EmptyResult{Error: dto.Failure("vaultUnavailable", "desktop", "never", 0)}
	}
	if !validUserTokenVaultRuntimeID(r.RuntimeID) {
		return dto.EmptyResult{Error: dto.Failure("invalidInput", "desktop", "never", 0)}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := p.Vault.ForgetUserToken(ctx, r.RuntimeID); err != nil {
		return dto.EmptyResult{Error: dto.Failure("vaultUnavailable", "desktop", "readOnly", 0)}
	}
	return dto.EmptyResult{OK: true, Value: &dto.Empty{}}
}

func validUserTokenVaultRuntimeID(value string) bool {
	return value != "" && len(value) <= 256 &&
		!strings.ContainsAny(value, "\x00\r\n/?#") &&
		value != "." && value != ".."
}

func userTokenVaultStatus(status vault.UserTokenStatus) dto.UserTokenVaultStatus {
	out := dto.UserTokenVaultStatus{
		Exists:    status.Exists,
		RuntimeID: status.RuntimeID,
		Issuer:    status.Issuer,
		ClientID:  status.ClientID,
		Subject:   status.Subject,
		MPAID:     status.MPAID,
		SpaceID:   status.SpaceID,
		ExpiresAt: status.ExpiresAt,
	}
	if status.UpdatedAt > 0 {
		out.UpdatedAt = time.UnixMilli(status.UpdatedAt).UTC().Format(time.RFC3339)
	}
	return out
}
