package e2e

import (
	"testing"
	"time"

	"github.com/na3shkw/abs-gpodder-bridge/e2e/internal/fake"
	"github.com/na3shkw/abs-gpodder-bridge/e2e/internal/harness"
)

func TestConfig_MissingRequiredEnvExitsWith1(t *testing.T) {
	for _, name := range []string{"GOPODDER_URL", "ABS_URL", "ABS_TOKEN"} {
		t.Run(name, func(t *testing.T) {
			gp, abs := fake.New(t), fake.New(t)
			env := baseEnv(gp, abs)
			delete(env, name)
			env["LISTEN_PORT"] = "0"

			code, _ := harness.RunToExit(t, env, 5*time.Second)
			if code == -1 {
				harness.Fatalf(t, "2.1", "%s が未設定なのに、ブリッジが終了しませんでした", name)
			}
			if code != 1 {
				harness.Failf(t, "2.1", "%s が未設定のときの終了コードが 1 ではありません（%d）", name, code)
			}
		})
	}
}
