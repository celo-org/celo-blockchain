// +build windows,386

package bls

/*
#cgo LDFLAGS: -L../target/i686-pc-windows-gnu/release -lepoch_snark -lm -lws2_32 -luserenv -lunwind
*/
import "C"
