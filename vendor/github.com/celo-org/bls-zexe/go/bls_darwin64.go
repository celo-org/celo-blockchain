// +build darwin,amd64

package bls

/*
#cgo LDFLAGS: -L../bls/target/x86_64-apple-darwin/release -L../bls/target/release -lbls_zexe -lbls_snark -ldl -lm
*/
import "C"
