package update

// TrustKeyForTesting makes the binary also trust key, a minisign public
// key, until restore is called. Only tests call it; nothing at run time
// can change which keys are trusted.
func TrustKeyForTesting(key string) (restore func()) {
	old := extraKey
	extraKey = key
	return func() { extraKey = old }
}
