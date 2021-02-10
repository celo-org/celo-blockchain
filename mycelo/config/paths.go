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
	return path.Join(p.Workdir, "env.json")
}

func (p Paths) GenesisConfig() string {
	return path.Join(p.Workdir, "genesis-config.json")
}

func (p Paths) ValidatorDatadir(idx int) string {
	return path.Join(p.Workdir, fmt.Sprintf("validator-%02d", idx))
}
