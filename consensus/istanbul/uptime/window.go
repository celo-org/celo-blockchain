package uptime

func newWindowEndingAt(end uint64, size uint64) Window {
	// being inclusive range math then size is:
	// size = end - start + 1
	// and thus start is:
	// start = end - size + 1
	return Window{
		Start: end - size + 1,
		End:   end,
	}
}

func newWindowStartingAt(start uint64, size uint64) Window {
	// being inclusive range math then size is:
	// size = end - start + 1
	// and thus end is:
	// end = start + size - 1
	return Window{
		Start: start,
		End:   start + size - 1,
	}
}

// Window represents a block range related to uptime monitoring
// Block range goes from `Start` to `End` and it's inclusive
type Window struct {
	Start uint64
	End   uint64
}

// Size returns the size of the window.
func (w Window) Size() uint64 { return w.End - w.Start + 1 }

// Contains indicates if a block number is contained within the window
func (w Window) Contains(n uint64) bool { return n <= w.End && n >= w.Start }
