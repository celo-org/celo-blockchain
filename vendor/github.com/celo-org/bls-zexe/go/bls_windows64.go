// +build windows,amd64

package bls

/*
#cgo LDFLAGS: -L../bls/target/x86_64-pc-windows-gnu/release -lbls_zexe -lbls_snark -lm -lws2_32 -luserenv -lunwind
*/
import "C"
