<script lang="ts">
  // The console chapter: real product shots in a tilted stack with scroll parallax, then the
  // feature wall and the stats row. Unpinned; t is this section's progress through the viewport.
  import fleet from './assets/console-fleet.webp'
  import machine from './assets/console-machine.webp'
  import changes from './assets/console-changes.webp'
  import search from './assets/console-package-search.webp'

  let { t }: { t: number } = $props()

  const FEATURES: [string, string][] = [
    ['Fleet graph', 'Machines, services, shared modules, packages, users and flake inputs: one map you can read.'],
    ['Edit by clicking', 'Add a package or set an option; it lands in that machine’s managed.json. Hand-written Nix is never rewritten.'],
    ['Checked before commit', 'nix flake check runs on exactly what is staged, and every machine it changes is built. Nothing commits, nothing pushes, until they pass.'],
    ['Changes in NixOS terms', '“homeserver: +jellyfin, port tcp/8096”, not a diff you have to decode.'],
    ['History and restore', 'Every version of every file, one click from the working copy.'],
    ['Agents via MCP', 'Automation uses the same actions as you, and cannot skip the checks.'],
  ]
  const STATS: [string, string][] = [
    ['1', 'repository'],
    ['every', 'machine evaluated'],
    ['0', 'unchecked pushes'],
    ['1 click', 'restore'],
  ]
  // Parallax: the layers drift at different speeds as the section crosses the viewport.
  const drift = $derived((t - 0.5) * 2)
</script>

<div class="console">
  <div class="shots" style:--d={drift}>
    <figure class="shot main">
      <img src={fleet} width="1280" height="800" alt="The fleet graph: homeserver, workstation and pi on the left, each linked to the services and programs it runs" loading="lazy" decoding="async" />
      <figcaption class="mono">fleet · services &amp; programs</figcaption>
    </figure>
    <figure class="shot side a">
      <img src={machine} width="1280" height="800" alt="A machine page: workstation, with its packages, services, users and the files that configure it" loading="lazy" decoding="async" />
      <figcaption class="mono">machines / workstation</figcaption>
    </figure>
    <figure class="shot side b">
      <img src={changes} width="1280" height="800" alt="The Changes view: neovim and spotify added to homeserver and workstation, and the diff of each machine's managed.json" loading="lazy" decoding="async" />
      <figcaption class="mono">changes · before commit</figcaption>
    </figure>
    <figure class="shot side c">
      <img src={search} width="1280" height="800" alt="Package search: results from nixpkgs, added to a machine with one click" loading="lazy" decoding="async" />
      <figcaption class="mono">packages · search nixpkgs</figcaption>
    </figure>
  </div>

  <ul class="wall">
    {#each FEATURES as [name, line] (name)}
      <li>
        <h3>{name}</h3>
        <p>{line}</p>
      </li>
    {/each}
  </ul>

  <dl class="stats">
    {#each STATS as [n, label] (label)}
      <div>
        <dt class="n">{n}</dt>
        <dd class="mono">{label}</dd>
      </div>
    {/each}
  </dl>
</div>

<style>
  .console {
    display: grid;
    gap: clamp(64px, 10vh, 140px);
  }

  /* ---- product shots ---- */
  .shots {
    --d: 0;
    position: relative;
    height: clamp(420px, 62vw, 760px);
    perspective: 1600px;
    perspective-origin: 50% 40%;
  }
  .shot {
    position: absolute;
    margin: 0;
    background: var(--bg-raised);
    border: 1px solid var(--line);
    box-shadow: var(--shadow-3);
    transform-style: preserve-3d;
    will-change: transform;
  }
  .shot img {
    display: block;
    width: 100%;
    height: auto;
    aspect-ratio: 1280 / 800;
  }
  .shot figcaption {
    position: absolute;
    top: -26px;
    left: 0;
    color: var(--i-muted);
    font-size: 0.8rem;
    font-weight: 600;
  }
  .main {
    left: 6%;
    top: 4%;
    width: 64%;
    transform: rotateY(-9deg) rotateX(3deg) translate3d(0, calc(var(--d) * -30px), 0);
  }
  .side {
    width: 40%;
  }
  .a {
    right: 2%;
    top: 24%;
    transform: rotateY(-14deg) rotateX(2deg) translate3d(0, calc(var(--d) * -70px), 60px);
  }
  .b {
    right: 10%;
    bottom: -2%;
    transform: rotateY(-12deg) translate3d(0, calc(var(--d) * -110px), 110px);
  }
  .c {
    left: 1%;
    bottom: 6%;
    width: 28%;
    transform: rotateY(10deg) translate3d(0, calc(var(--d) * -40px), 40px);
  }

  /* ---- feature wall ---- */
  .wall {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 1px;
    margin: 0;
    padding: 0;
    list-style: none;
    background: var(--i-line);
    border: 1px solid var(--i-line);
  }
  .wall li {
    display: grid;
    align-content: start;
    gap: 14px;
    min-height: 220px;
    padding: clamp(22px, 2.6vw, 36px);
    background: var(--i-bg);
  }
  .wall h3 {
    font-weight: 700;
    font-size: 1.15rem;
  }
  .wall p {
    max-width: 34ch;
    color: var(--i-muted);
    font-size: 0.95rem;
    line-height: 1.5;
    text-wrap: pretty;
  }

  /* ---- stats ---- */
  .stats {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: 32px;
    margin: 0;
    padding-top: 28px;
    border-top: 1px solid var(--i-line);
  }
  .stats div {
    display: grid;
    gap: 8px;
  }
  .stats dt {
    font-weight: 800;
    font-size: clamp(2.6rem, 5.4vw, 5.6rem);
    line-height: 0.95;
    letter-spacing: -0.035em;
  }
  .stats dd {
    margin: 0;
    color: var(--i-muted);
    font-size: 0.875rem;
  }

  @media (max-width: 860px) {
    .shots {
      height: auto;
      display: grid;
      gap: 40px;
      perspective: none;
    }
    .shot {
      position: static;
      width: 100%;
      transform: none;
      transform-style: flat;
    }
    .side.c {
      display: none;
    }
    .wall {
      grid-template-columns: 1fr;
    }
    .wall li {
      min-height: 0;
    }
    .stats {
      grid-template-columns: repeat(2, 1fr);
    }
  }
</style>
