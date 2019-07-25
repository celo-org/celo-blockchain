// +build !linux,!darwin,android
// +build arm64

package bls

/*
#cgo LDFLAGS: -L../bls/target/aarch64-linux-android/release -lbls_zexe -ldl -lm
*/
import "C"
