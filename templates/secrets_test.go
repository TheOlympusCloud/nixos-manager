package templates

import (
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

// The machine web, with a key, as the starter fleet builds it next to pi, without one.
const secretsMachines = `
let
  nixpkgs = builtins.getFlake "github:NixOS/nixpkgs/nixos-26.05";
  inputs = {
    sops-nix = builtins.getFlake "github:Mic92/sops-nix";
    comin = builtins.getFlake "github:nlewo/comin";
    home-manager = builtins.getFlake "github:nix-community/home-manager/release-26.05";
  };
  machine =
    name:
    nixpkgs.lib.nixosSystem {
      system = "x86_64-linux";
      specialArgs = { inherit inputs; };
      modules = [
        ./modules/common.nix
        inputs.comin.nixosModules.comin
        inputs.home-manager.nixosModules.home-manager
        {
          boot.loader.grub.enable = false;
          fileSystems."/" = {
            device = "nodev";
            fsType = "tmpfs";
          };
          system.stateVersion = "26.05";
          networking.hostName = name;
          # common.nix leaves it null; the reporter must be on to see its token file.
          fleet.deployReport.url = nixpkgs.lib.mkForce "http://manager.example.invalid:8080";
        }
      ];
    };
  show = m: {
    secrets = builtins.mapAttrs (_: s: {
      file = baseNameOf s.sopsFile;
      inherit (s) format path;
    }) m.config.sops.secrets;
    tokenFile = m.config.fleet.deployReport.tokenFile;
    cominToken = (builtins.head m.config.services.comin.remotes).auth.access_token_path;
    hostKeys = m.config.services.openssh.generateHostKeys;
    ageKeys = m.config.sops.age.sshKeyPaths;
    # Evaluates every assertion and sops-nix's manifest.
    system = builtins.isString m.config.system.build.toplevel.drvPath;
  };
in
{
  web = show (machine "web");
  pi = show (machine "pi");
}
`

// modules/secrets.nix declares every secret of common.json and the machine's
// own file once the machine has a key (its own value wins), points comin and the
// report hook at theirs, and declares nothing for a machine without a key.
func TestSecretsModule(t *testing.T) {
	if os.Getenv("NIXMGR_NIX_TESTS") == "" {
		t.Skip("needs real Nix and network access: set NIXMGR_NIX_TESTS=1")
	}
	dir := t.TempDir()
	modules, err := fs.Sub(Fleet, "modules")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(filepath.Join(dir, "modules"), modules); err != nil {
		t.Fatal(err)
	}
	sops := `,"sops":{"age":[{"recipient":"age1x","enc":"x"}],"mac":"x","version":"3.13.3"}}`
	for p, body := range map[string]string{
		"secrets/common.json":    `{"wifi":"ENC[a]","comin-token":"ENC[b]"` + sops,
		"secrets/hosts/web.json": `{"wifi":"ENC[c]","nixos-manager-report":"ENC[d]"` + sops,
		"secrets/hosts/web.pub":  "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOZAEbqZQ8qXzvnBHvAPVPAOiFvWWQkfdeRqsbJ7vFZl\n",
		"secrets/hosts/pi.json":  `{"other":"ENC[e]"` + sops, // pi has no key: ignored
		"machines.nix":           secretsMachines,
	} {
		os.MkdirAll(filepath.Dir(filepath.Join(dir, p)), 0o755)
		if err := os.WriteFile(filepath.Join(dir, p), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out, err := exec.Command("nix", "--extra-experimental-features", "nix-command flakes",
		"eval", "--impure", "--json", "--file", filepath.Join(dir, "machines.nix")).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("nix eval: %v\n%s", err, ee.Stderr)
		}
		t.Fatal(err)
	}
	var got map[string]map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	secret := func(file, name string) map[string]any {
		return map[string]any{"file": file, "format": "json", "path": "/run/secrets/" + name}
	}
	wantWeb := map[string]any{
		"secrets": map[string]any{
			"wifi":                 secret("web.json", "wifi"),
			"comin-token":          secret("common.json", "comin-token"),
			"nixos-manager-report": secret("web.json", "nixos-manager-report"),
		},
		"tokenFile":  "/run/secrets/nixos-manager-report",
		"cominToken": "/run/secrets/comin-token",
		"hostKeys":   true,
		"ageKeys":    []any{"/etc/ssh/ssh_host_ed25519_key"},
		"system":     true,
	}
	if !reflect.DeepEqual(got["web"], wantWeb) {
		t.Errorf("web:\n got %v\nwant %v", got["web"], wantWeb)
	}
	wantPi := map[string]any{
		"secrets":    map[string]any{},
		"tokenFile":  "/var/lib/nixos-manager-report/token",
		"cominToken": "",
		"hostKeys":   true,
		"ageKeys":    []any{"/etc/ssh/ssh_host_ed25519_key"},
		"system":     true,
	}
	if !reflect.DeepEqual(got["pi"], wantPi) {
		t.Errorf("pi:\n got %v\nwant %v", got["pi"], wantPi)
	}
}
