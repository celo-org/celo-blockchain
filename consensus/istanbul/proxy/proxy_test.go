package proxy_test

import (
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/consensus/istanbul/backend"
	"github.com/ethereum/go-ethereum/consensus/istanbul/backend/backendtest"
)

func TestMain(m *testing.M) {
	backendtest.InitTestBackendFactory(backend.TestBackendFactory)
	code := m.Run()
	os.Exit(code)
}
