//go:build !windows

package atomicfile

import "os"

func replace(source string, target string) error {
	return os.Rename(source, target)
}
