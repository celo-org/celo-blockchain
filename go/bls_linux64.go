// +build linux,amd64

package bls

/*
#cgo LDFLAGS: -L../target/x86_64-unknown-linux-gnu/release -lbls_crypto -lepoch_snark -ldl -lm
*/
import "C"
