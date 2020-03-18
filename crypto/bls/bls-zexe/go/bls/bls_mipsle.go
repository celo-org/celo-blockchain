// +build linux,mipsle

package bls

/*
#cgo LDFLAGS: -L../../target/mipsel-unknown-linux-gnu/release -lepoch_snark -ldl -lm
*/
import "C"
