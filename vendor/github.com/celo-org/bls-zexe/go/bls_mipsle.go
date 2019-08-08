// +build linux,mipsle

package bls

/*
#cgo LDFLAGS: -L../bls/target/mipsel-unknown-linux-gnu/release -lbls_zexe -ldl -lm
*/
import "C"
