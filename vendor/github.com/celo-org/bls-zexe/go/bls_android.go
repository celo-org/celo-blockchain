// +build android

package bls

/*
#cgo LDFLAGS: -L../bls/target/x86_64-linux-android/release -L../bls/target/i686-linux-android/release -L../bls/target/armv7-linux-androideabi/release -L../bls/target/aarch64-linux-android/release -lbls_zexe -lbls_snark -ldl -lm
*/
import "C"
