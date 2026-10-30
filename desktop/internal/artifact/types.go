// Package artifact owns the two native save flows. Its IPC requests contain
// remote references or page text, never renderer-selected local filesystem paths.
package artifact

import "github.com/superops-team/OpenMPA/desktop/internal/dto"

type DownloadRequest struct {
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	SessionID       string `json:"sessionId"`
	RemotePath      string `json:"remotePath"`
}
type DownloadProgress struct {
	BytesWritten int64  `json:"bytesWritten"`
	TotalBytes   *int64 `json:"totalBytes,omitempty"`
}
type DownloadReceipt struct {
	Saved        bool  `json:"saved"`
	BytesWritten int64 `json:"bytesWritten"`
	// length-verified means only that Content-Length matched. It does not
	// prove a digest, authenticity, or the semantic completeness of a file.
	Integrity string `json:"integrity"`
}
type DownloadResult struct {
	OK    bool             `json:"ok"`
	Value *DownloadReceipt `json:"value"`
	Error *dto.SafeError   `json:"error"`
}
type ExportMarkdownRequest struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}
type ExportReceipt struct {
	Saved        bool  `json:"saved"`
	BytesWritten int64 `json:"bytesWritten"`
}
type ExportResult struct {
	OK    bool           `json:"ok"`
	Value *ExportReceipt `json:"value"`
	Error *dto.SafeError `json:"error"`
}
