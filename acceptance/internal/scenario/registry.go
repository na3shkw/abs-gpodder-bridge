package scenario

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
)

// モードは環境変数 DIAGRAMS_MODE で決める。
//
//	（なし） テストを実行する
//	write   ブリッジを起動せず、図と一覧ページを diagrams/ に書き出す
//	check   ブリッジを起動せず、diagrams/ が最新かを確かめる（違えば失敗）
const (
	modeRun   = ""
	modeWrite = "write"
	modeCheck = "check"
)

// DiagramDir は、パッケージのディレクトリから見た図の置き場所。
const DiagramDir = "diagrams"

func mode() string { return os.Getenv("DIAGRAMS_MODE") }

// DiagramMode は、図の生成モード（テストを実行しない）なら true を返す。
func DiagramMode() bool { return mode() != modeRun }

type entry struct {
	id       string
	title    string
	testName string
	svg      string
	file     string
	line     int
}

var (
	regMu    sync.Mutex
	registry []*entry
)

func register(s *S) {
	if strings.Contains(s.t.Name(), "/") {
		s.t.Errorf("scenario %s: サブテストでは使えません（テスト 1 件に図 1 枚のため）", s.id)
		return
	}
	regMu.Lock()
	defer regMu.Unlock()
	registry = append(registry, &entry{id: s.id, title: s.title, testName: s.t.Name(), svg: s.d.SVG()})
}

// Main は TestMain から呼ぶ。図の生成モードなら、テストのあとで図を書き出す・確かめる。
func Main(m *testing.M) {
	code := m.Run()
	if DiagramMode() && code == 0 {
		if err := finish(mode()); err != nil {
			fmt.Fprintln(os.Stderr, "diagrams:", err)
			code = 1
		}
	}
	os.Exit(code)
}

func finish(md string) error {
	if md != modeWrite && md != modeCheck {
		return fmt.Errorf("DIAGRAMS_MODE は write か check です（%q）", md)
	}
	funcs, err := scanTests(".")
	if err != nil {
		return err
	}
	var problems []string
	seen := map[string]bool{}
	sort.Slice(registry, func(i, j int) bool { return registry[i].id < registry[j].id })
	for _, e := range registry {
		if seen[e.id] {
			problems = append(problems, e.id+": ID が重複しています")
		}
		seen[e.id] = true
		fn, ok := funcs[e.testName]
		if !ok {
			problems = append(problems, e.testName+": テスト関数が見つかりません")
			continue
		}
		e.file, e.line = fn.file, fn.line
		want := "図: " + DiagramDir + "/" + e.id + ".svg"
		if !strings.Contains(fn.doc, want) {
			problems = append(problems, fmt.Sprintf("%s:%d %s: 直前のコメントに「%s」がありません", fn.file, fn.line, e.testName, want))
		}
	}
	for name, fn := range funcs {
		if fn.usesScenario && !hasTest(name) {
			problems = append(problems, fmt.Sprintf("%s:%d %s: scenario を使っていますが、図が登録されませんでした（Run を呼んでいない？）", fn.file, fn.line, name))
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("\n  %s", strings.Join(problems, "\n  "))
	}

	files := map[string][]byte{}
	for _, e := range registry {
		files[e.id+".svg"] = []byte(e.svg)
	}
	files["index.html"] = []byte(indexHTML(registry))

	existing, _ := filepath.Glob(filepath.Join(DiagramDir, "*"))
	if md == modeWrite {
		if err := os.MkdirAll(DiagramDir, 0o755); err != nil {
			return err
		}
		for _, p := range existing {
			if _, keep := files[filepath.Base(p)]; !keep {
				if err := os.Remove(p); err != nil {
					return err
				}
			}
		}
		for name, b := range files {
			if err := os.WriteFile(filepath.Join(DiagramDir, name), b, 0o644); err != nil {
				return err
			}
		}
		fmt.Fprintf(os.Stderr, "diagrams: %d 件の図と index.html を書き出しました\n", len(registry))
		return nil
	}
	var stale []string
	for name, b := range files {
		got, err := os.ReadFile(filepath.Join(DiagramDir, name))
		if err != nil || !bytes.Equal(got, b) {
			stale = append(stale, name)
		}
	}
	for _, p := range existing {
		if _, ok := files[filepath.Base(p)]; !ok {
			stale = append(stale, filepath.Base(p)+"（対応するテストがない）")
		}
	}
	if len(stale) > 0 {
		sort.Strings(stale)
		return fmt.Errorf("図が最新ではありません。harness/bin/diagrams で書き出してください:\n  %s", strings.Join(stale, "\n  "))
	}
	fmt.Fprintf(os.Stderr, "diagrams: %d 件の図は最新です\n", len(registry))
	return nil
}

func hasTest(name string) bool {
	for _, e := range registry {
		if e.testName == name {
			return true
		}
	}
	return false
}

type testFunc struct {
	file         string
	line         int
	doc          string
	usesScenario bool
}

// scanTests は *_test.go を読み、テスト関数の位置・直前のコメント・scenario の使用有無を返す。
func scanTests(dir string) (map[string]testFunc, error) {
	fset := token.NewFileSet()
	paths, _ := filepath.Glob(filepath.Join(dir, "*_test.go"))
	out := map[string]testFunc{}
	for _, p := range paths {
		f, err := parser.ParseFile(fset, p, nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Recv != nil || !strings.HasPrefix(fd.Name.Name, "Test") || fd.Name.Name == "TestMain" {
				continue
			}
			uses := false
			ast.Inspect(fd, func(n ast.Node) bool {
				if sel, ok := n.(*ast.SelectorExpr); ok {
					if id, ok := sel.X.(*ast.Ident); ok && id.Name == "scenario" && sel.Sel.Name == "New" {
						uses = true
					}
				}
				return true
			})
			out[fd.Name.Name] = testFunc{
				file:         filepath.Base(p),
				line:         fset.Position(fd.Pos()).Line,
				doc:          fd.Doc.Text(),
				usesScenario: uses,
			}
		}
	}
	return out, nil
}
