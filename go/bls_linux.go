// +build !android

package bls

/*
#cgo LDFLAGS: -L../target/release -lbls_crypto -lepoch_snark -ldl -lm
*/
import "C"
