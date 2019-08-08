// +build linux,arm,!arm7

package bls

/*
#cgo LDFLAGS: -L../bls/target/arm-unknown-linux-gnueabi/release -lbls_zexe -ldl -lm
*/
import "C"
