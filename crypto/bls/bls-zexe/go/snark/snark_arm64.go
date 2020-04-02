// +build linux,arm64

package snark

/*
#cgo LDFLAGS: -L../../target/aarch64-unknown-linux-gnu/release -lepoch_snark -ldl -lm
*/
import "C"

