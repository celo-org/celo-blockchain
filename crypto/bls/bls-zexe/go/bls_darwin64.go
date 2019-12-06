// +build darwin,amd64

package bls

/*
#cgo LDFLAGS: -L../bls/target/release -L../bls/target/x86_64-apple-darwin/release -lbls_zexe -lbls_snark -ldl -lm
*/
import "C"
