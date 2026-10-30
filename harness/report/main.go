// Command report は go test -json の出力を、実装者に渡してよい形に整形する。
//
// 実装者に渡すのは次の情報だけとする。
//   - テスト名と合否
//   - テストが harness.Failf などで出した "REPORT: " 行（仕様の節番号と短い失敗理由）
//
// テストのソース位置、期待値、ブリッジのログなど、それ以外の出力はすべて捨てる。
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
)

const reportMarker = "REPORT: "

type event struct {
	Action  string `json:"Action"`
	Package string `json:"Package"`
	Test    string `json:"Test"`
	Output  string `json:"Output"`
}

type testResult struct {
	Name     string   `json:"name"`
	Status   string   `json:"status"` // pass / fail / skip
	Messages []string `json:"messages,omitempty"`
}

type result struct {
	ID       string       `json:"id"`
	Status   string       `json:"status"` // pass / fail / build_error / error
	Run      string       `json:"run,omitempty"`
	Passed   int          `json:"passed"`
	Failed   int          `json:"failed"`
	Skipped  int          `json:"skipped"`
	Tests    []testResult `json:"tests,omitempty"`
	BuildLog string       `json:"build_log,omitempty"`
	Message  string       `json:"message,omitempty"`
}

func main() {
	id := flag.String("id", "", "run id")
	run := flag.String("run", "", "go test -run pattern")
	buildStatus := flag.Int("build-status", 0, "exit status of go build")
	buildLog := flag.String("build-log", "", "path to build log")
	testJSON := flag.String("test-json", "", "path to go test -json output")
	outJSON := flag.String("out-json", "", "output path (json)")
	outMD := flag.String("out-md", "", "output path (markdown)")
	flag.Parse()

	r := result{ID: *id, Run: *run}
	if *buildStatus != 0 {
		r.Status = "build_error"
		r.Message = "実装のビルドに失敗しました。"
		// ビルドログは実装コード自身のエラーなので、そのまま返してよい
		r.BuildLog = tail(readFile(*buildLog), 80)
	} else {
		parseTests(&r, *testJSON)
	}

	writeJSON(*outJSON, r)
	writeFile(*outMD, renderMarkdown(r))
}

func parseTests(r *result, path string) {
	f, err := os.Open(path)
	if err != nil {
		r.Status = "error"
		r.Message = "テスト結果を読み取れませんでした。"
		return
	}
	defer f.Close()

	tests := map[string]*testResult{}
	order := []string{}
	pkgFailed := false
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		var ev event
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		if ev.Test == "" {
			if ev.Action == "fail" {
				pkgFailed = true
			}
			continue
		}
		tr, ok := tests[ev.Test]
		if !ok {
			tr = &testResult{Name: ev.Test}
			tests[ev.Test] = tr
			order = append(order, ev.Test)
		}
		switch ev.Action {
		case "output":
			if i := strings.Index(ev.Output, reportMarker); i >= 0 {
				tr.Messages = append(tr.Messages, strings.TrimSpace(ev.Output[i+len(reportMarker):]))
			}
		case "pass", "fail", "skip":
			tr.Status = ev.Action
		}
	}

	sort.SliceStable(order, func(i, j int) bool { return order[i] < order[j] })
	for _, name := range order {
		tr := tests[name]
		if tr.Status == "" {
			// 終了イベントがない＝パニックかタイムアウトで中断された
			tr.Status = "fail"
			tr.Messages = append(tr.Messages, "テストが異常終了しました（パニックまたはタイムアウト）。")
		}
		if tr.Status == "fail" && len(tr.Messages) == 0 && !hasFailedSubtest(name, tests) {
			tr.Messages = []string{"失敗しました（詳細は非公開）。"}
		}
		switch tr.Status {
		case "pass":
			r.Passed++
		case "fail":
			r.Failed++
		case "skip":
			r.Skipped++
		}
		r.Tests = append(r.Tests, *tr)
	}

	switch {
	case len(r.Tests) == 0 && pkgFailed:
		// テストコード自体のビルド失敗など。内容はテストを含むので返さない
		r.Status = "error"
		r.Message = "テストハーネス側でエラーが発生しました。利用者に確認を依頼してください。"
	case len(r.Tests) == 0:
		r.Status = "error"
		r.Message = "実行されたテストがありません（-run のパターンを確認してください）。"
	case r.Failed > 0 || pkgFailed:
		r.Status = "fail"
	default:
		r.Status = "pass"
	}
}

func hasFailedSubtest(name string, tests map[string]*testResult) bool {
	for n, t := range tests {
		if strings.HasPrefix(n, name+"/") && t.Status == "fail" {
			return true
		}
	}
	return false
}

func renderMarkdown(r result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# 隠蔽テスト結果 %s\n\n", r.ID)
	fmt.Fprintf(&b, "status: **%s**", r.Status)
	if r.Run != "" {
		fmt.Fprintf(&b, "（-run %s）", r.Run)
	}
	b.WriteString("\n\n")
	if r.Message != "" {
		b.WriteString(r.Message + "\n\n")
	}
	if r.BuildLog != "" {
		b.WriteString("## ビルドログ\n\n```\n" + r.BuildLog + "\n```\n")
		return b.String()
	}
	if len(r.Tests) == 0 {
		return b.String()
	}
	fmt.Fprintf(&b, "passed %d / failed %d / skipped %d\n\n", r.Passed, r.Failed, r.Skipped)
	if r.Failed > 0 {
		b.WriteString("## 失敗\n\n")
		for _, t := range r.Tests {
			if t.Status != "fail" {
				continue
			}
			fmt.Fprintf(&b, "- `%s`\n", t.Name)
			for _, m := range t.Messages {
				fmt.Fprintf(&b, "  - %s\n", m)
			}
		}
		b.WriteString("\n")
	}
	b.WriteString("## 成功\n\n")
	for _, t := range r.Tests {
		if t.Status == "pass" {
			fmt.Fprintf(&b, "- `%s`\n", t.Name)
		}
	}
	return b.String()
}

func tail(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func readFile(path string) string {
	b, _ := os.ReadFile(path)
	return string(b)
}

func writeJSON(path string, v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	writeFile(path, string(b)+"\n")
}

func writeFile(path, s string) {
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
