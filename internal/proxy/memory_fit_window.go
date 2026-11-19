package proxy

import "sync"

type memoryFitWindow struct {
	disturbed bool
}

type memoryFitWindows struct {
	mu   sync.Mutex
	open map[*memoryFitWindow]struct{}
}

func newMemoryFitWindows() *memoryFitWindows {
	return &memoryFitWindows{open: map[*memoryFitWindow]struct{}{}}
}

func (windows *memoryFitWindows) begin() *memoryFitWindow {
	windows.mu.Lock()
	defer windows.mu.Unlock()
	window := &memoryFitWindow{disturbed: len(windows.open) > 0}
	windows.disturbAllLocked()
	windows.open[window] = struct{}{}
	return window
}

func (windows *memoryFitWindows) runtimeChanged() {
	windows.mu.Lock()
	defer windows.mu.Unlock()
	windows.disturbAllLocked()
}

func (windows *memoryFitWindows) finish(window *memoryFitWindow) bool {
	windows.mu.Lock()
	defer windows.mu.Unlock()
	delete(windows.open, window)
	return !window.disturbed
}

func (windows *memoryFitWindows) disturbAllLocked() {
	for window := range windows.open {
		window.disturbed = true
	}
}
