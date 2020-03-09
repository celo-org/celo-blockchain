// +build linux,mipsle

package bls

/*
#cgo LDFLAGS: -L../target/mipsel-unknown-linux-gnu/release -lbls_crypto -lepoch_snark -ldl -lm
*/
import "C"
