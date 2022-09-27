package types

func (h *Header) IstanbulExtra() (*IstanbulExtra, error) {
	h.extraLock.Lock()
	defer h.extraLock.Unlock()

	extraData := h.deserializedExtra

	if extraData == nil {
		extra, err := extractIstanbulExtra(h)
		if err != nil {
			return nil, err
		}
		h.deserializedExtra = extra
	}

	return h.deserializedExtra, nil
}
