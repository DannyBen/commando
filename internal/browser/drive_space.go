package browser

// DriveSpace is one caller-visible snapshot; Used is Total minus Free.
// It is separate from recursive file sizes and is never persisted.
type DriveSpace struct {
	Free, Total int64
}
