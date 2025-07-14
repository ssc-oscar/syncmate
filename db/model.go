package db

import (
	"gorm.io/gorm"
)

type Status int

const (
	Pending Status = iota
	Uploading
	Uploaded
	Downloading
	Downloaded
	Failed
)

type Task struct {
	gorm.Model
	/* VirtualPath is the path in the S3 bucket and the virual file system.
	   It is unique and not null. */
	VirtualPath string `gorm:"uniqueIndex;not null"`
	/* SrcPath is the path of the file in the transfer source. */
	SrcPath string `gorm:"not null"`
	/* SrcSize is the size of the file in the transfer source. */
	SrcSize int64 `gorm:"not null"`
	/* SrcDigest is the sample_md5 digest of the file in the transfer source. */
	SrcDigest string `gorm:"not null"`
	/* DstPath is the path of the file in the transfer destination. */
	DstPath string `gorm:"not null"`
	/* DstSize is the size of the file in the transfer destination. */
	DstSize int64 `gorm:"not null"`
	/* Status is the status of the task. */
	Status Status `gorm:"not null"`
	/* Error is the error message of the task. */
	Error string `gorm:"type:text"`
}
