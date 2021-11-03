package announce

import "sync/atomic"

type Version interface {
	Get() uint
	Set(version uint)
}

type VersionReader interface {
	Get() uint
}

type atomicVersion struct {
	v atomic.Value
}

func NewAtomicVersion() Version {
	return &atomicVersion{}
}

func (av *atomicVersion) Get() uint {
	vuint := av.v.Load()
	if vuint == nil {
		return 0
	}
	return vuint.(uint)
}

func (av *atomicVersion) Set(version uint) {
	av.v.Store(version)
}
