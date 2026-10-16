// Package harness は、e2e テストからブリッジを起動・検証するための補助を提供する。
package harness

import (
	"fmt"
	"testing"
)

// 実装者に返す結果には、"REPORT: " で始まる行だけが残る（harness/report を参照）。
// 失敗理由は、仕様の節番号と「何が起きなかったか」だけを書く。
// 期待値の具体的な値や、テストの入力データは書かないこと。

// Failf は、実装者に見せてよい失敗理由を記録してテストを失敗にする。
// spec は仕様の節（例："5.1-4"）。
func Failf(t testing.TB, spec, format string, args ...any) {
	t.Helper()
	t.Errorf("REPORT: [§%s] %s", spec, fmt.Sprintf(format, args...))
}

// Fatalf は Failf と同じ内容を記録して、テストを直ちに終了する。
func Fatalf(t testing.TB, spec, format string, args ...any) {
	t.Helper()
	t.Fatalf("REPORT: [§%s] %s", spec, fmt.Sprintf(format, args...))
}
