//go:build unix

package proxy

import "syscall"

const openNoFollow = syscall.O_NOFOLLOW
