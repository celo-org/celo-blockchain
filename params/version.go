// Copyright 2016 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package params

import (
	"fmt"
)

// On master, the version should be the UPCOMING one and "unstable"
// e.g. if the latest release was v1.3.2, master should be 1.4.0-unstable
// On release branches, it should be a beta or stable.  For example:
// "1.3.0-beta", "1.3.0-beta.2", etc. and then "1.3.0-stable", "1.3.1-stable", etc.
const (
	VersionMajor = 1        // Major version component of the current release
	VersionMinor = 5        // Minor version component of the current release
	VersionPatch = 1        // Patch version component of the current release
	VersionMeta  = "stable" // Version metadata to append to the version string
)

type VersionInfo struct {
	Major uint64
	Minor uint64
	Patch uint64
}

// Cmp compares x and y and returns:
//
//   -1 if x <  y
//    0 if x == y
//   +1 if x >  y
//
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

// Version holds the textual version string.
var Version = func() string {
	return fmt.Sprintf("%d.%d.%d", VersionMajor, VersionMinor, VersionPatch)
}()

var CurrentVersionInfo = func() *VersionInfo {
	return &VersionInfo{VersionMajor, VersionMinor, VersionPatch}
}()

// VersionWithMeta holds the textual version string including the metadata.
var VersionWithMeta = func() string {
	v := Version
	if VersionMeta != "" {
		v += "-" + VersionMeta
	}
	return v
}()

// ArchiveVersion holds the textual version string used for Geth archives.
// e.g. "1.8.11-dea1ce05" for stable releases, or
//      "1.8.13-unstable-21c059b6" for unstable releases
func ArchiveVersion(gitCommit string) string {
	vsn := Version
	if VersionMeta != "stable" {
		vsn += "-" + VersionMeta
	}
	if len(gitCommit) >= 8 {
		vsn += "-" + gitCommit[:8]
	}
	return vsn
}

func VersionWithCommit(gitCommit, gitDate string) string {
	vsn := VersionWithMeta
	if len(gitCommit) >= 8 {
		vsn += "-" + gitCommit[:8]
	}
	if (VersionMeta != "stable") && (gitDate != "") {
		vsn += "-" + gitDate
	}
	return vsn
}
