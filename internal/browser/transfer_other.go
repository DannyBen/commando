//go:build !windows

package browser

func transferNative(request TransferRequest, _ []string, _ *transferBatchTrace) (TransferResult, error) {
	return TransferResult{Directory: request.Directory}, ErrTransferUnavailable
}
