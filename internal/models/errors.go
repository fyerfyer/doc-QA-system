package models

import "errors"

var (
	// ErrDocumentNotFound 文档不存在错误
	ErrDocumentNotFound = errors.New("document not found")

	// ErrInvalidDocumentStatus 无效的文档状态错误
	ErrInvalidDocumentStatus = errors.New("invalid document status")
)
