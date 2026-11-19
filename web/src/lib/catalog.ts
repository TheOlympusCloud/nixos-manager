// Ready-made NixOS services a machine can get by clicking. Each recipe only sets
// scalar options: managed settings win over hand-written ones, so a recipe never
// writes a list or an attribute set that would replace what the code defines.

export type Recipe = {
  id: string
  name: string
  category: 'Media' | 'Files' | 'Home' | 'Containers' | 'Monitoring' | 'Network' | 'Desktop' | 'Data' | 'AI'
  summary: string
  /** Where to open it once deployed, when it has a web page. */
  url?: string
  /** The first setting is the one that says whether the recipe is on. */
  settings: { path: string[]; value: string | number | boolean }[]
}

const on = (...path: string[]) => ({ path, value: true })

export const recipes: Recipe[] = [
  { id: 'jellyfin', name: 'Jellyfin', category: 'Media', url: 'http://<machine>:8096',
    summary: 'Stream your films, series and music to any device.',
    settings: [on('services', 'jellyfin', 'enable'), on('services', 'jellyfin', 'openFirewall')] },
  { id: 'immich', name: 'Immich', category: 'Media', url: 'http://<machine>:2283',
    summary: 'Photo and video backup from your phone, with search and albums.',
    settings: [on('services', 'immich', 'enable'), on('services', 'immich', 'openFirewall')] },
  { id: 'syncthing', name: 'Syncthing', category: 'Files',
    summary: 'Keep folders in sync between machines, peer to peer. Its web page listens on localhost:8384.',
    settings: [on('services', 'syncthing', 'enable'), on('services', 'syncthing', 'openDefaultPorts')] },
  { id: 'samba', name: 'Samba', category: 'Files',
    summary: 'Windows and macOS file sharing. Define the shares in code.',
    settings: [on('services', 'samba', 'enable'), on('services', 'samba', 'openFirewall')] },
  { id: 'home-assistant', name: 'Home Assistant', category: 'Home', url: 'http://<machine>:8123',
    summary: 'Home automation for lights, sensors and ESPHome devices.',
    settings: [on('services', 'home-assistant', 'enable'), on('services', 'home-assistant', 'openFirewall')] },
  { id: 'adguardhome', name: 'AdGuard Home', category: 'Network', url: 'http://<machine>:3000',
    summary: 'Network-wide ad and tracker blocking DNS server.',
    settings: [on('services', 'adguardhome', 'enable'), on('services', 'adguardhome', 'openFirewall')] },
  { id: 'avahi', name: 'mDNS (Avahi)', category: 'Network',
    summary: 'Reach the machine as <name>.local and discover printers and shares.',
    settings: [on('services', 'avahi', 'enable'), on('services', 'avahi', 'nssmdns4'), on('services', 'avahi', 'openFirewall')] },
  { id: 'docker', name: 'Docker', category: 'Containers',
    summary: 'The Docker engine. Add users to the docker group to use it without sudo.',
    settings: [on('virtualisation', 'docker', 'enable')] },
  { id: 'podman', name: 'Podman', category: 'Containers',
    summary: 'Daemonless containers, with a docker-compatible command. Not alongside Docker.',
    settings: [on('virtualisation', 'podman', 'enable'), on('virtualisation', 'podman', 'dockerCompat')] },
  { id: 'libvirtd', name: 'Virtual machines', category: 'Containers',
    summary: 'KVM virtual machines with libvirt and the virt-manager app.',
    settings: [on('virtualisation', 'libvirtd', 'enable'), on('programs', 'virt-manager', 'enable')] },
  { id: 'cockpit', name: 'Cockpit', category: 'Monitoring', url: 'https://<machine>:9090',
    summary: 'A web page to watch the machine: logs, services, storage and a terminal.',
    settings: [on('services', 'cockpit', 'enable'), on('services', 'cockpit', 'openFirewall')] },
  { id: 'node-exporter', name: 'Prometheus node exporter', category: 'Monitoring',
    summary: 'CPU, memory, disk and network metrics on port 9100 for Prometheus.',
    settings: [on('services', 'prometheus', 'exporters', 'node', 'enable'), on('services', 'prometheus', 'exporters', 'node', 'openFirewall')] },
  { id: 'smartd', name: 'Disk health (smartd)', category: 'Monitoring',
    summary: 'Watches the disks’ SMART data and logs failing drives early.',
    settings: [on('services', 'smartd', 'enable')] },
  { id: 'postgresql', name: 'PostgreSQL', category: 'Data',
    summary: 'The PostgreSQL database, reachable locally.',
    settings: [on('services', 'postgresql', 'enable')] },
  { id: 'ollama', name: 'Ollama', category: 'AI',
    summary: 'Run language models locally; its API listens on localhost:11434.',
    settings: [on('services', 'ollama', 'enable')] },
  { id: 'open-webui', name: 'Open WebUI', category: 'AI', url: 'http://<machine>:8080',
    summary: 'A chat page for the models Ollama serves.',
    settings: [on('services', 'open-webui', 'enable'), on('services', 'open-webui', 'openFirewall')] },
  { id: 'flatpak', name: 'Flatpak', category: 'Desktop',
    summary: 'Install desktop apps from Flathub next to Nix packages.',
    settings: [on('services', 'flatpak', 'enable')] },
  { id: 'bluetooth', name: 'Bluetooth', category: 'Desktop',
    summary: 'Bluetooth devices, powered on at boot.',
    settings: [on('hardware', 'bluetooth', 'enable'), on('hardware', 'bluetooth', 'powerOnBoot')] },
  { id: 'fwupd', name: 'Firmware updates', category: 'Desktop',
    summary: 'Update BIOS and device firmware from LVFS with fwupdmgr.',
    settings: [on('services', 'fwupd', 'enable')] },
]

export const categories = [...new Set(recipes.map((r) => r.category))]
