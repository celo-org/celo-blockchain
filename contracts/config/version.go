package config

import "github.com/celo-org/celo-blockchain/params"

type VersionInfo struct {
	Major uint64
	Minor uint64
	Patch uint64
}

// Cmp compares x and y and returns:
//
//	-1 if x <  y
//	 0 if x == y
//	+1 if x >  y
func cmp(x uint64, y uint64) int {
	if x < y {
		return -1
	}
	if x > y {
		return 1
	}
	return 0
}

func (v *VersionInfo) Cmp(version *VersionInfo) int {
	if v.Major == version.Major {
		if v.Minor == version.Minor {
			return cmp(v.Patch, version.Patch)
		}
		return cmp(v.Minor, version.Minor)
	}
	return cmp(v.Major, version.Major)
}

var CurrentVersionInfo = func() *VersionInfo {
	return &VersionInfo{params.VersionMajor, params.VersionMinor, params.VersionPatch}
}()
