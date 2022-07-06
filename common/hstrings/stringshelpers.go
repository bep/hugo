package hstrings

// CommonPrefix returns the longest common prefix of the given strings.
// This can be made considerably faster, see https://go-review.googlesource.com/c/go/+/408116/3/src/strings/common.go
func CommonPrefix(a, b string) string {
	commonLen := len(a)
	if len(b) < commonLen {
		commonLen = len(b)
	}
	var i int
	for i = 0; i < commonLen && a[i] == b[i]; i++ {
	}
	return a[:i]
}
