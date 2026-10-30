// Package harness は、e2e テストからブリッジを起動するための補助を提供する。
package harness

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"
)

// 環境変数:
//   BRIDGE_BIN  テスト対象のブリッジのバイナリ（必須）

// Bridge は起動中のブリッジのプロセスを表す。
type Bridge struct {
	URL  string // 例: http://127.0.0.1:12345
	Port int

	cmd  *exec.Cmd
	out  *syncBuffer
	done chan struct{}
}

// Env はブリッジに渡す環境変数。
type Env map[string]string

// Start はブリッジを起動し、/healthz が応答するまで待つ。
// LISTEN_PORT が指定されていなければ空きポートを割り当てる。
// 終了はテストの Cleanup で行う。
func Start(t testing.TB, env Env) *Bridge {
	t.Helper()
	b, code, ok := TryStart(t, env, true)
	if !ok {
		if code >= 0 {
			t.Fatalf("ブリッジが起動直後に終了しました（終了コード %d）", code)
		}
		t.Fatal("10 秒以内に GET /healthz が応答しませんでした")
	}
	return b
}

// SpecDefaultListenPort は、仕様で決めた LISTEN_PORT の既定値。
const SpecDefaultListenPort = 8080

// TryStart はブリッジを起動し、/healthz が応答するか、プロセスが終了するまで待つ（最大 10 秒）。
// 応答したら (b, -1, true)、終了したら (nil, 終了コード, false)、どちらでもなければ (nil, -1, false)。
// autoPort が true で LISTEN_PORT が指定されていなければ空きポートを割り当てる。false なら仕様の既定値に接続する。
func TryStart(t testing.TB, env Env, autoPort bool) (*Bridge, int, bool) {
	t.Helper()
	port := SpecDefaultListenPort
	if p, ok := env["LISTEN_PORT"]; ok {
		port, _ = strconv.Atoi(p)
	} else if autoPort {
		env = clone(env)
		port = FreePort(t)
		env["LISTEN_PORT"] = strconv.Itoa(port)
	}
	b := spawn(t, env)
	b.Port = port
	b.URL = fmt.Sprintf("http://127.0.0.1:%d", port)

	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-b.done:
			return nil, b.cmd.ProcessState.ExitCode(), false
		default:
		}
		resp, err := client.Get(b.URL + "/healthz")
		if err == nil {
			resp.Body.Close()
			return b, -1, true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil, -1, false
}

// RunToExit はブリッジを起動し、終了するまで待って終了コードを返す。
// timeout までに終了しなければ -1 を返す。
func RunToExit(t testing.TB, env Env, timeout time.Duration) (exitCode int, output string) {
	t.Helper()
	b := spawn(t, env)
	select {
	case <-b.done:
		return b.cmd.ProcessState.ExitCode(), b.out.String()
	case <-time.After(timeout):
		return -1, b.out.String()
	}
}

// Output はブリッジが標準出力・標準エラーに出した内容を返す。
func (b *Bridge) Output() string { return b.out.String() }

func spawn(t testing.TB, env Env) *Bridge {
	t.Helper()
	if os.Getenv("E2E_DIAGRAMS") != "" {
		t.Skip("図の生成モードではブリッジを起動しない")
	}
	bin := os.Getenv("BRIDGE_BIN")
	if bin == "" {
		t.Fatal("BRIDGE_BIN が設定されていません。リポジトリの直下で scripts/e2e を実行してください")
	}
	cmd := exec.CommandContext(context.Background(), bin)
	cmd.Dir = "/"
	// 環境変数はテストで指定したものだけを渡す
	cmd.Env = []string{"PATH=/usr/bin:/bin", "TZ=UTC"}
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	out := &syncBuffer{}
	cmd.Stdout = out
	cmd.Stderr = out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("ブリッジを起動できません: %v", err)
	}
	b := &Bridge{cmd: cmd, out: out, done: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		close(b.done)
	}()
	t.Cleanup(func() {
		select {
		case <-b.done:
		default:
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
			select {
			case <-b.done:
			case <-time.After(3 * time.Second):
				_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
				<-b.done
			}
		}
		if t.Failed() {
			// 失敗したときだけ、ブリッジの出力をテストのログに残す
			t.Logf("bridge output:\n%s", out.String())
		}
	})
	return b
}

// FreePort は空いている TCP ポートを返す。
func FreePort(t testing.TB) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func clone(env Env) Env {
	c := Env{}
	for k, v := range env {
		c[k] = v
	}
	return c
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}
