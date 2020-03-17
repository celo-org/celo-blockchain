// +build windows,amd64

package bls

/*
#cgo LDFLAGS: -L../../target/x86_64-pc-windows-gnu/release -L../../target/release -lepoch_snark -lm -lws2_32 -luserenv -lunwind
*/
import "C"

