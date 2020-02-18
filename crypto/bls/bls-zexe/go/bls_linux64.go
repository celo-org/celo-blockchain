// +build linux,amd64

package bls

/*
#cgo LDFLAGS: -L../bls/target/x86_64-unknown-linux-gnu/release -lbls_zexe -lbls_snark -ldl -lm
*/
import "C"
