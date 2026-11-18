package model

type PresignedURLRequest struct {
	FileName    string `json:"file_name" validate:"required,max=255"`
	FileType    string `json:"file_type" validate:"required,max=100"`
	FileSize    int64  `json:"file_size" validate:"required,gt=0"`
	Description string `json:"description" validate:"omitempty,max=2000"`
}

type PresignedURLResponse struct {
	PresignedURL string `json:"presigned_url"`
	FileID       string `json:"file_id"`
	ObjectKey    string `json:"object_key"`
	ExpiresIn    int    `json:"expires_in"`
}

// ConfirmUploadRequest marks a pending upload as complete and optionally
// attaches it to its owning record through the generic source_table /
// source_table_id pair (plus arbitrary extra metadata), leazi-style. The
// storage key is resolved server-side from the pending record; the client
// does not supply it.
type ConfirmUploadRequest struct {
	FileID        string                 `json:"file_id" validate:"required,uuid"`
	SourceTable   string                 `json:"source_table" validate:"omitempty,max=100"`
	SourceTableID string                 `json:"source_table_id" validate:"omitempty,max=100"`
	Description   string                 `json:"description" validate:"omitempty,max=2000"`
	Extra         map[string]interface{} `json:"extra"`
}

// FileResponse is the response model for file details (leazi-style).
type FileResponse struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	FilePath      string `json:"file_path"`
	FileType      string `json:"file_type"`
	FileSize      int64  `json:"file_size"`
	URL           string `json:"url,omitempty"`
	SourceTable   string `json:"source_table,omitempty"`
	SourceTableID string `json:"source_table_id,omitempty"`
	Description   string `json:"description,omitempty"`
}
