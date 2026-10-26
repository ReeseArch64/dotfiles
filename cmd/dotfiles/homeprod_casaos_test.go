package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestHomeProdCasaOSInstallerFailures(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"success", "fallback", "bad-checksum", "download-failure", "installer-failure"} {
		t.Run(scenario, func(t *testing.T) {
			bin := t.TempDir()
			work := t.TempDir()
			t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
			t.Setenv("TEST_WORK", work)
			t.Setenv("SCENARIO", scenario)
			writeTestExecutable(t, bin, "id", `echo 0`)
			writeTestExecutable(t, bin, "mktemp", `printf '%s\n' "$TEST_WORK"`)
			writeTestExecutable(t, bin, "infocmp", `exit 1`)
			writeTestExecutable(t, bin, "sha256sum", `cat > "$TEST_WORK/checksum-input"
[ "$SCENARIO" != bad-checksum ]`)
			fixture := `#!/bin/bash
readonly CASA_UNINSTALL_URL="https://get.casaos.io/uninstall/v0.4.16"
[[ "$CASA_UNINSTALL_URL" == "file://$TEST_WORK/uninstall.sh" ]] || exit 90
[[ "$TERM" == xterm ]] || exit 91
touch "$TEST_WORK/executed"
echo 'installer output'
[[ "$SCENARIO" != installer-failure ]] || exit 22
`
			if err := os.WriteFile(filepath.Join(work, "fixture"), []byte(fixture), 0600); err != nil {
				t.Fatal(err)
			}
			writeTestExecutable(t, bin, "curl", `url="${@: -3:1}"
output="${@: -1}"
if [[ "$url" == https://get.casaos.io ]]; then
  [[ "$SCENARIO" != download-failure ]] || exit 22
  cp "$TEST_WORK/fixture" "$output"
elif [[ "$url" == https://get.casaos.io/uninstall/v0.4.16 ]]; then
  [[ "$SCENARIO" != fallback && "$SCENARIO" != bad-checksum ]] || exit 22
  printf '#!/bin/bash\nexit 99\n' > "$output"
else
  printf '#!/bin/bash\nexit 99\n' > "$output"
fi`)
			output, err := exec.Command("bash", filepath.Join(root, "setup/homeprod/casaos/install.sh")).CombinedOutput()
			success := scenario == "success" || scenario == "fallback"
			if (err == nil) != success {
				t.Fatalf("err=%v output=%s", err, output)
			}
			_, executedErr := os.Stat(filepath.Join(work, "executed"))
			if (executedErr == nil) != (success || scenario == "installer-failure") {
				t.Fatalf("unexpected execution: %s", output)
			}
			if !success && !strings.Contains(string(output), "Falha na etapa:") {
				t.Fatalf("missing error context: %s", output)
			}
			if scenario == "fallback" || scenario == "bad-checksum" {
				if _, err := os.Stat(filepath.Join(work, "checksum-input")); err != nil {
					t.Fatal("checksum not verified")
				}
			}
		})
	}
}
