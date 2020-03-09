// +build ios

package bls

/*
#cgo LDFLAGS: -L../target/universal/release -lbls_crypto -lepoch_snark -ldl -lm -framework Security -framework Foundation
*/
import "C"
