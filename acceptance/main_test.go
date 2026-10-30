package acceptance

import (
	"testing"

	"github.com/na3shkw/abs-gpodder-bridge/acceptance/internal/scenario"
)

// TestMain は、図の生成モード（DIAGRAMS_MODE=write / check）のとき、テストのあとで図を書き出す・確かめる。
func TestMain(m *testing.M) { scenario.Main(m) }
