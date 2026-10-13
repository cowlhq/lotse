// Package deploy holds what installs lotse: the image, the hub's compose
// file, and (in site/) the install script.
package deploy_test

import (
	"os"
	"strings"
	"testing"
)

// The install script writes the hub's compose file from a copy of its own;
// it must be the same as deploy/compose.yml.
func TestInstallScriptComposeFile(t *testing.T) {
	compose, err := os.ReadFile("compose.yml")
	if err != nil {
		t.Fatal(err)
	}
	script, err := os.ReadFile("../site/install")
	if err != nil {
		t.Fatal(err)
	}
	start := "cat >/opt/lotse/compose.yml <<'EOF'\n"
	i := strings.Index(string(script), start)
	if i < 0 {
		t.Fatal("the install script writes no compose file")
	}
	rest := string(script)[i+len(start):]
	embedded := rest[:strings.Index(rest, "EOF\n")]
	if embedded != string(compose) {
		t.Error("the compose file in site/install differs from deploy/compose.yml; copy it over")
	}
}
