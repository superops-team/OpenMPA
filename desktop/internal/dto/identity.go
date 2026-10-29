package dto

type IdentityPrewarmRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	IDToken         string `json:"idToken"`
	RefreshToken    string `json:"refreshToken"`
}

type IdentityPrewarm struct {
	SenderID string `json:"senderId"`
}
