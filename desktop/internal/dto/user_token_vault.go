package dto

type UserTokenVaultStatus struct {
	Exists     bool   `json:"exists"`
	RuntimeID  string `json:"runtimeId,omitempty"`
	UserPoolID string `json:"userPoolId,omitempty"`
	Issuer     string `json:"issuer,omitempty"`
	ClientID   string `json:"clientId,omitempty"`
	Subject    string `json:"subject,omitempty"`
	MPAID      string `json:"mpaId,omitempty"`
	SpaceID    string `json:"spaceId,omitempty"`
	ExpiresAt  string `json:"expiresAt,omitempty"`
	UpdatedAt  string `json:"updatedAt,omitempty"`
}

type UserTokenVaultRequest struct {
	RuntimeID string `json:"runtimeId"`
}

type UserTokenVaultUnlockRequest struct {
	RuntimeID       string `json:"runtimeId"`
	Passphrase      string `json:"passphrase"`
	ConnectionEpoch uint64 `json:"connectionEpoch,omitempty"`
}

type UserTokenVaultSaveRequest struct {
	Passphrase   string `json:"passphrase"`
	RuntimeID    string `json:"runtimeId"`
	UserPoolID   string `json:"userPoolId,omitempty"`
	Issuer       string `json:"issuer"`
	ClientID     string `json:"clientId"`
	Subject      string `json:"subject"`
	MPAID        string `json:"mpaId"`
	SpaceID      string `json:"spaceId"`
	IDToken      string `json:"idToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresAt    string `json:"expiresAt"`
}

type AgentIdentityConfig struct {
	UserPoolID          string `json:"userPoolId,omitempty"`
	SuggestedUserPoolID string `json:"suggestedUserPoolId,omitempty"`
}

type AgentIdentityConfigRequest struct {
	UserPoolID string `json:"userPoolId"`
}
