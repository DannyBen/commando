//go:build !windows

package browser

func transferNative(request TransferRequest, _ []string) (TransferResult, error) {
	return TransferResult{Directory: request.Directory}, ErrTransferUnavailable
}
