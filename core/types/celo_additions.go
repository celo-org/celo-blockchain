package types

// IstanbulExtra returns the 'Extra' field of the header deserialized into an
// IstanbulExtra struct, if there is an error deserializing the 'Extra' field
// it will be returned.
func (h *Header) IstanbulExtra() (*IstanbulExtra, error) {
	h.extraLock.Lock()
	defer h.extraLock.Unlock()

	if h.extraValue == nil && h.extraError == nil {
		extra, err := extractIstanbulExtra(h)

		h.extraValue = extra
		h.extraError = err
	}

	return h.extraValue, h.extraError
}
