// +build linux,mips64

package bls

/*
#cgo LDFLAGS: -L../bls/target/mips64-unknown-linux-gnu/release -lbls_zexe -ldl -lm
*/
import "C"
