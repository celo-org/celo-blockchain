// +build linux,amd64

package snark

/*
#cgo LDFLAGS: -L../../target/x86_64-unknown-linux-gnu/release -lepoch_snark -ldl -lm
*/
import "C"
