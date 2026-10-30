package e2e

import (
	"testing"

	"github.com/na3shkw/abs-gpodder-bridge/e2e/internal/scenario"
)

// TestMain は、図の生成モード（E2E_DIAGRAMS=write / check）のとき、テストのあとで図を書き出す・確かめる。
func TestMain(m *testing.M) { scenario.Main(m) }
