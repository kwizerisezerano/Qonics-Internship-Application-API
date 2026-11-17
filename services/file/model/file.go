package model

type PresignedURLRequest struct {
	FileName string `json:"file_name" validate:"required"`
	FileType string `json:"file_type" validate:"required"`
	FileSize int64  `json:"file_size" validate:"required"`
}

type PresignedURLResponse struct {
	PresignedURL string `json:"presigned_url"`
	FileID       string `json:"file_id"`
	ObjectKey    string `json:"object_key"`
	ExpiresIn    int    `json:"expires_in"`
}

type ConfirmUploadRequest struct {
	FileID    string `json:"file_id" validate:"required"`
	ObjectKey string `json:"object_key" validate:"required"`
}
