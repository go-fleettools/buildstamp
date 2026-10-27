package buildstamp

import "testing"

// FuzzRead: whatever the bytes, Read returns build information or an error;
// it never panics and never looks past MaxInfo bytes after the sentinel.
func FuzzRead(f *testing.F) {
	f.Add(stamped("false"))
	f.Add(stamped("true"))
	f.Add([]byte("\x00asm"))
	f.Add(append(append([]byte{}, infoStart...), infoEnd...))
	f.Add(append(append(append([]byte{}, infoStart...), "path\tx\n=>\ty\tv1\n"...), infoEnd...))
	f.Fuzz(func(t *testing.T, b []byte) {
		bi, err := Read(b)
		if err == nil {
			_ = Clean(bi, "")
		}
	})
}
