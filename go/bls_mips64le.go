// +build linux,mips64le

package bls

/*
#cgo LDFLAGS: -L../target/mips64el-unknown-linux-gnu/release -lepoch_snark -ldl -lm
*/
import "C"
