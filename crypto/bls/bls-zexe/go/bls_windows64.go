// +build windows,amd64

package bls

/*
#cgo LDFLAGS: -L../target/x86_64-pc-windows-gnu/release -lbls_crypto -lepoch_snark -lm -lws2_32 -luserenv -lunwind
*/
import "C"
