// +build windows

package evtx

func GetNativeResolver() (MessageResolver, error) {
	return NewWindowsMessageResolver(), nil
}
