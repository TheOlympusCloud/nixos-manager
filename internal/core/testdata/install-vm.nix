# A machine installed from its installer ISO, in QEMU: boots the ISO nix/iso.nix
# builds, runs fleet-install --yes --offline on an empty disk, boots that disk,
# logs in on the console as user and types command. What command prints is
# $out/session.txt. Run by TestInstallInVMWithRealNix (KVM needed).
{
  flakeRef,
  host,
  iso, # nix/iso.nix's result for host
  user,
  password,
  command,
}:
let
  fleet = builtins.getFlake flakeRef;
  target = fleet.nixosConfigurations.${host};
  cfg = target.config;
  inherit (target) pkgs;
  inherit (pkgs) lib;
  # Every flake input's source, so that the fleet evaluates without the network.
  sources = flake: lib.concatMap (i: [ i.outPath ] ++ sources i) (builtins.attrValues (flake.inputs or { }));
  installer =
    (iso.extendModules {
      modules = [
        # The test driver's shell on the ISO.
        (pkgs.path + "/nixos/modules/testing/test-instrumentation.nix")
        {
          # The VM has no network: what the install needs is on the ISO already.
          isoImage.storeContents = [
            cfg.system.build.toplevel
            cfg.system.build.diskoScript
          ]
          ++ lib.unique (sources fleet);
          isoImage.squashfsCompression = "zstd -Xcompression-level 1"; # quicker to build
          nix.settings = {
            substituters = lib.mkForce [ ];
            # nixos-install copies the system into /mnt's store, but builds what has
            # allowSubstitutes = false (system-path, the system itself…), which needs stdenv.
            always-allow-substitutes = true;
          };
        }
      ];
    }).config.system.build.isoImage;
  qemu = (import (pkgs.path + "/nixos/lib/qemu-common.nix") { inherit (pkgs) lib stdenv; }).qemuBinary pkgs.qemu_test;
  # The same disk for both: in the machine's state folder, kept between the two.
  start = flags: "${qemu} -m 4096 -smp 2 -drive file=disk.qcow2,if=virtio ${flags}";
  py = builtins.toJSON; # a Python string
in
pkgs.testers.runNixOSTest {
  name = "fleet-install";
  nodes = { };
  testScript = ''
    import os
    import subprocess

    # The driver stops only its nodes: on a failure, these would keep it waiting.
    installer = create_machine(${py (start "-boot d -cdrom ${installer}/iso/${installer.isoName}")}, name=${py host})
    try:
        subprocess.run(["${pkgs.qemu_test}/bin/qemu-img", "create", "-f", "qcow2", str(installer.state_dir / "disk.qcow2"), "10G"], check=True)
        installer.start()
        installer.wait_for_unit("multi-user.target")
        with subtest("fleet-install --yes --offline on the empty disk"):
            # It ends with a reboot, which would stop QEMU under the driver: the test shuts down instead.
            installer.succeed("mkdir -p /tmp/stub && printf '#!/bin/sh\\n' > /tmp/stub/reboot && chmod +x /tmp/stub/reboot")
            installer.succeed("PATH=/tmp/stub:$PATH fleet-install --yes --offline < /dev/null >&2")
        installer.shutdown()
    finally:
        installer.release()

    machine = create_machine(${py (start "")}, name=${py host}, keep_machine_state=True)
    try:
        machine.start()
        machine.wait_for_unit("multi-user.target")
        machine.wait_for_unit(${py "home-manager-${user}.service"})
        with subtest(${py "${user} logs in"}):
            machine.wait_until_tty_matches("1", "login: ")
            machine.send_chars(${py "${user}\n"})
            machine.wait_until_tty_matches("1", "Password: ")
            machine.send_chars(${py "${password}\n"})
            machine.wait_until_succeeds(${py "pgrep -u ${user} bash"})
            machine.send_chars(${py "(${command}) > /tmp/session 2>&1; mv /tmp/session /tmp/session.txt\n"})
            machine.wait_for_file("/tmp/session.txt")
        with open(os.environ["out"] + "/session.txt", "w") as f:
            f.write(machine.succeed("cat /tmp/session.txt"))
        machine.shutdown()
    finally:
        machine.release()
  '';
}
