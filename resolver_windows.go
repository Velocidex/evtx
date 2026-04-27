//go:build windows
// +build windows

package evtx

func GetNativeResolver(opts MessageResolverOpts) (MessageResolver, error) {
	return NewWindowsMessageResolver(opts)
}
