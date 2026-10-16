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
//   BRIDGE_UID  ブリッジを起動するユーザー ID（任意）。隠蔽テストの実行時は nobody を指定する
//   BRIDGE_GID  同じくグループ ID

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
	if _, ok := env["LISTEN_PORT"]; !ok {
		env = clone(env)
		env["LISTEN_PORT"] = strconv.Itoa(FreePort(t))
	}
	port, _ := strconv.Atoi(env["LISTEN_PORT"])
	b := spawn(t, env)
	b.Port = port
	b.URL = fmt.Sprintf("http://127.0.0.1:%d", port)

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-b.done:
			Fatalf(t, "2.1", "ブリッジが起動直後に終了しました（終了コード %d）", b.cmd.ProcessState.ExitCode())
		default:
		}
		resp, err := http.Get(b.URL + "/healthz")
		if err == nil {
			resp.Body.Close()
			return b
		}
		time.Sleep(50 * time.Millisecond)
	}
	Fatalf(t, "2.3", "10 秒以内に GET /healthz が応答しませんでした")
	return nil
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
	bin := os.Getenv("BRIDGE_BIN")
	if bin == "" {
		t.Fatal("BRIDGE_BIN が設定されていません")
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
	if uid := os.Getenv("BRIDGE_UID"); uid != "" {
		u, _ := strconv.Atoi(uid)
		g, _ := strconv.Atoi(os.Getenv("BRIDGE_GID"))
		cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uint32(u), Gid: uint32(g)}
	}
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
			// 生ログにだけ残る（実装者には返さない）
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
