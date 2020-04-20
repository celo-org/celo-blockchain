// +build linux,arm64

package bls

/*
#cgo LDFLAGS: -L../../target/aarch64-unknown-linux-gnu/release -lepoch_snark -ldl -lm
*/
import "C"

