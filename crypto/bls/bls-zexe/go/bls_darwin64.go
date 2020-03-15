// +build darwin,amd64

package bls

/*
#cgo LDFLAGS: -L../target/x86_64-apple-darwin/release -L../target/release -lepoch_snark -ldl -lm
*/
import "C"
