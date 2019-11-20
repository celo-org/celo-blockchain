// +build windows,386

package bls

/*
#cgo LDFLAGS: -L../bls/target/i686-pc-windows-gnu/release -lbls_zexe -lbls_snark -lm -lws2_32 -luserenv -lunwind
*/
import "C"
