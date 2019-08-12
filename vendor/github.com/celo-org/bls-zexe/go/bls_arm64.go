// +build linux,arm64

package bls

/*
#cgo LDFLAGS: -L../bls/target/aarch64-unknown-linux-gnu/release -lbls_zexe -ldl -lm
*/
import "C"
