package dto

type PlatformStateResult struct {
	OK    bool           `json:"ok"`
	Value *PlatformState `json:"value,omitempty"`
	Error *SafeError     `json:"error,omitempty"`
}
type RegionListResult struct {
	OK    bool        `json:"ok"`
	Value *RegionList `json:"value,omitempty"`
	Error *SafeError  `json:"error,omitempty"`
}
type VaultStatusResult struct {
	OK    bool         `json:"ok"`
	Value *VaultStatus `json:"value,omitempty"`
	Error *SafeError   `json:"error,omitempty"`
}
type RuntimeSummaryResult struct {
	OK    bool            `json:"ok"`
	Value *RuntimeSummary `json:"value,omitempty"`
	Error *SafeError      `json:"error,omitempty"`
}
type RuntimeListResult struct {
	OK    bool         `json:"ok"`
	Value *RuntimeList `json:"value,omitempty"`
	Error *SafeError   `json:"error,omitempty"`
}
type VersionListResult struct {
	OK    bool         `json:"ok"`
	Value *VersionList `json:"value,omitempty"`
	Error *SafeError   `json:"error,omitempty"`
}
type ConnectionStateResult struct {
	OK    bool             `json:"ok"`
	Value *ConnectionState `json:"value,omitempty"`
	Error *SafeError       `json:"error,omitempty"`
}
type ConnectionTestResult struct {
	OK    bool            `json:"ok"`
	Value *ConnectionTest `json:"value,omitempty"`
	Error *SafeError      `json:"error,omitempty"`
}
type SessionSummaryResult struct {
	OK    bool            `json:"ok"`
	Value *SessionSummary `json:"value,omitempty"`
	Error *SafeError      `json:"error,omitempty"`
}
type SessionListResult struct {
	OK    bool         `json:"ok"`
	Value *SessionList `json:"value,omitempty"`
	Error *SafeError   `json:"error,omitempty"`
}
type SessionHistoryResult struct {
	OK    bool            `json:"ok"`
	Value *SessionHistory `json:"value,omitempty"`
	Error *SafeError      `json:"error,omitempty"`
}
type DeleteReceiptResult struct {
	OK    bool           `json:"ok"`
	Value *DeleteReceipt `json:"value,omitempty"`
	Error *SafeError     `json:"error,omitempty"`
}
type RunReceiptResult struct {
	OK    bool        `json:"ok"`
	Value *RunReceipt `json:"value,omitempty"`
	Error *SafeError  `json:"error,omitempty"`
}
type AbortReceiptResult struct {
	OK    bool          `json:"ok"`
	Value *AbortReceipt `json:"value,omitempty"`
	Error *SafeError    `json:"error,omitempty"`
}
type ExecutionConfigResult struct {
	OK    bool             `json:"ok"`
	Value *ExecutionConfig `json:"value,omitempty"`
	Error *SafeError       `json:"error,omitempty"`
}
type PendingListResult struct {
	OK    bool         `json:"ok"`
	Value *PendingList `json:"value,omitempty"`
	Error *SafeError   `json:"error,omitempty"`
}
type EmptyResult struct {
	OK    bool       `json:"ok"`
	Value *Empty     `json:"value,omitzero"`
	Error *SafeError `json:"error,omitempty"`
}
