package config

import (
	"fmt"
	"path"
)

type Paths struct {
	Workdir string
	Geth    string
}

func (p Paths) GenesisJSON() string {
	return path.Join(p.Workdir, "genesis.json")
}

func (p Paths) Config() string {
	return path.Join(p.Workdir, "config.json")
}

func (p Paths) ContractsConfig() string {
	return path.Join(p.Workdir, "contracts-config.json")
}

func (p Paths) ValidatorDatadir(idx int) string {
	return path.Join(p.Workdir, fmt.Sprintf("validator-%02d", idx))
}
