package uptime

func newWindowEndingOn(end uint64, size uint64) Window {
	// being incluse range math then size is:
	// size = end - start + 1
	// and thus start is:
	// start = end - size + 1
	return Window{
		Start: end - size + 1,
		End:   end,
	}
}

func newWindowStartingOn(start uint64, size uint64) Window {
	// being incluse range math then size is:
	// size = end - start + 1
	// and thus end is:
	// end = start + size - 1
	return Window{
		Start: start,
		End:   start + size - 1,
	}
}

// Window tipyfies a block range related to uptime monitoring
type Window struct {
	Start uint64
	End   uint64
}

// Size returns the size of the window.
// Window block ranges are inclusive
func (w Window) Size() uint64 { return w.End - w.Start + 1 }

// Contains indidicates if a block number is contained within the window
func (w Window) Contains(n uint64) bool { return n <= w.End && n >= w.Start }
