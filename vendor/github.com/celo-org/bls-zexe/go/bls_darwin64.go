// +build darwin,amd64

package bls

/*
#cgo LDFLAGS: -L../bls/target/universal/release -L../bls/target/release -lbls_zexe -ldl -lm
*/
import "C"
