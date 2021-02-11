package env

import (
	"fmt"
	"path"
)

type paths struct {
	Workdir string
	Geth    string
}

func (p paths) genesisJSON() string {
	return path.Join(p.Workdir, "genesis.json")
}

func (p paths) envJson() string {
	return path.Join(p.Workdir, "env.json")
}

func (p paths) validatorDatadir(idx int) string {
	return path.Join(p.Workdir, fmt.Sprintf("validator-%02d", idx))
}
