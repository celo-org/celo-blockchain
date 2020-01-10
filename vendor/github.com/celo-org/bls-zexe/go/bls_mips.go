// +build linux,mips

package bls

/*
#cgo LDFLAGS: -L../bls/target/mips-unknown-linux-gnu/release -lbls_zexe -lbls_snark -ldl -lm
*/
import "C"
