// +build linux,mips64le

package bls

/*
#cgo LDFLAGS: -L../bls/target/mips64el-unknown-linux-gnu/release -lbls_zexe -lbls_snark -ldl -lm
*/
import "C"
