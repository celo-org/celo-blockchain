// +build arm7

package bls

/*
#cgo LDFLAGS: -L../target/arm-unknown-linux-gnueabihf/release -lbls_crypto -lepoch_snark -ldl -lm
*/
import "C"
