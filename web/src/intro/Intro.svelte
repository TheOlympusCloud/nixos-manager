<script lang="ts">
  // The intro: a scroll story in nine chapters over one text scene. This file owns the scrolling, the copy and the
  // layout; Scene.svelte paints the planet behind it, Blueprint.svelte the drawing.
  import { onMount } from 'svelte'
  import Icon from '../components/Icon.svelte'
  import { big, bigText, sweep, wipe } from '../lib/ascii'
  import Blueprint from './blueprint/Blueprint.svelte'
  import DeployRail from './DeployRail.svelte'
  import Headline from './Headline.svelte'
  import Scene from './Scene.svelte'
  import Showcase from './Showcase.svelte'
  import { STORY, countdown, counterLabel, formatGrouped, isSceneChapter, locate, type Position, type Section, type StoryChapter } from './story'
  import type { Chapter } from './three/types'

  let { onEnter, back }: { onEnter: () => void; back?: () => void } = $props()

  const MACHINES = ['workstation', 'homeserver', 'pi']
  const reducedMotion = matchMedia('(prefers-reduced-motion: reduce)').matches

  let root: HTMLElement
  const els = $state<Partial<Record<StoryChapter, HTMLElement>>>({})
  let sections: Section[] = []
  let pos = $state<Position>({ id: 'hero', t: 0, index: 0 })

  // The scene keeps the last scene chapter while the page draws the DOM-only ones.
  let sceneChapter = $state<Chapter>('hero')
  let sceneT = $state(0)
  const sceneVisible = $derived(isSceneChapter(pos.id))
  $effect(() => {
    if (isSceneChapter(pos.id)) {
      sceneChapter = pos.id
      sceneT = pos.t
    }
  })

  function measure() {
    sections = STORY.map((s) => ({ id: s.id, top: els[s.id]!.offsetTop, height: els[s.id]!.offsetHeight, pinned: s.len > 0 }))
    update()
  }
  function update() {
    if (!sections.length) return
    const p = locate(root.scrollTop, root.clientHeight, sections)
    if (p.id !== pos.id || p.t !== pos.t) pos = p
  }
  const progressOf = (i: number) => (pos.index === i ? pos.t : pos.index > i ? 1 : 0)
  // With reduced motion nothing scrubs: the scene, the counter, the drawing and the rail show their finished state.
  const scrub = (i: number) => (reducedMotion ? 1 : progressOf(i))
  // The counter runs over the second half of the assemble chapter, after the statement has landed.
  const counterT = (i: number) => (scrub(i) - 0.55) / 0.4

  // A chapter is a jump away, the way a terminal pages: no smooth scrolling off the clock.
  function scrollTo(id: StoryChapter) {
    const el = els[id]
    if (el) root.scrollTop = el.offsetTop
  }

  onMount(() => {
    measure()
    root.focus({ preventScroll: true })
    const ro = new ResizeObserver(measure)
    ro.observe(root.firstElementChild as Element)
    return () => ro.disconnect()
  })

  // Each chapter is repainted as it arrives: its copy is realised and its headline wipes in again.
  let arrived = -1
  $effect(() => {
    const i = pos.index
    // The stage, not the section: it is the part on screen, and the sweep runs top to bottom from its top.
    const stage = els[STORY[i].id]?.querySelector<HTMLElement>('.stage')
    if (i === arrived || !stage) return
    arrived = i
    sweep(stage, 5)
    for (const pre of stage.querySelectorAll<HTMLElement>('.headline pre.big')) wipe(pre, big(pre.getAttribute('aria-label') ?? ''))
  })

  const progress = $derived(
    `${String(pos.index + 1).padStart(2, '0')}/${String(STORY.length).padStart(2, '0')} ` +
      '━'.repeat(Math.round(((pos.index + pos.t) / STORY.length) * 24)).padEnd(24, '─'),
  )

  // Copy. The machine list mirrors the example fleet the console ships with.
  const FLEET = [
    ['workstation', 'x86_64-linux', 'desktop · plasma · firefox · steam'],
    ['homeserver', 'x86_64-linux', 'forgejo · jellyfin · nextcloud'],
    ['pi', 'aarch64-linux', 'home-assistant · mosquitto'],
  ]
  const DRIFT = [
    ['openssl', '3.0.13', '1.1.1w'],
    ['kernel', '6.12.58', '6.1.0'],
    ['postgresql', '16.4', '13.2 · pinned by hand'],
  ]
  const nav: [StoryChapter, string][] = [
    ['drift', 'The challenge'],
    ['assemble', 'The NixOS way'],
    ['blueprint', 'Blueprint'],
    ['console', 'Console'],
    ['deploy', 'Deploy'],
  ]
  const FEATURES: [string, string, string][] = [
    ['◈', 'Fleet graph', 'Machines, services, shared modules, packages, users and flake inputs: one map you can read.'],
    ['✎', 'Edit by clicking', 'Add a package or set an option; it lands in that machine’s managed.json. Hand-written Nix is never rewritten.'],
    ['▷', 'Checked before commit', 'nix flake check runs on exactly what is staged, and every machine it changes is built. Nothing commits, nothing pushes, until they pass.'],
    ['±', 'Changes in NixOS terms', '“homeserver: +jellyfin, port tcp/8096”, not a diff you have to decode.'],
    ['↺', 'History and restore', 'Every version of every file, one click from the working copy.'],
    ['λ', 'Agents via MCP', 'Automation uses the same actions as you, and cannot skip the checks.'],
  ]
  const STATS: [string, string][] = [
    ['1', 'repository'],
    ['every', 'machine evaluated'],
    ['0', 'unchecked pushes'],
    ['1 click', 'restore'],
  ]
</script>

<div class="intro" class:rm={reducedMotion} bind:this={root} onscroll={update} tabindex="-1">
  <div class="content">
    <!-- Without motion every chapter's scene shows its finished state; the hero's planet stays where it rises from, clear of the copy. -->
    <Scene chapter={sceneChapter} t={reducedMotion ? (sceneChapter === 'hero' ? 0 : 1) : sceneT} visible={sceneVisible} machines={MACHINES} />

    <header class="nav">
      <a class="brand" href="#top" onclick={(e) => (e.preventDefault(), scrollTo('hero'))} aria-label="nixos-manager, back to the top">
        <Icon name="mark" /> <span>nixos-manager</span>
      </a>
      <span class="spacer"></span>
      <nav class="links" aria-label="Chapters">
        {#each nav as [id, label] (id)}
          <a href="#{id}" class:on={pos.id === id} onclick={(e) => (e.preventDefault(), scrollTo(id))}>{label}</a>
        {/each}
      </nav>
      {#if back}
        <button class="primary" onclick={back}>Back to the console</button>
      {:else}
        <button class="primary" onclick={onEnter}>Enter the console</button>
      {/if}
    </header>

    <main>
      {#each STORY as s, i (s.id)}
        <section class="chapter {s.id}" class:flow={s.id === 'console' || s.id === 'outro'} id={s.id} bind:this={els[s.id]} style:--len={s.len || 1}>
          {#if s.id === 'hero'}
            <div class="stage">
              <div class="copy">
                <Headline tag="h1" lines={[['Every machine.'], ['One source', 'acc'], ['of truth.', 'acc']]} />
                <p class="lede">A self-hosted console for every NixOS machine you own: evaluated from one flake, drawn as one graph, checked before anything ships.</p>
                <div class="ctas">
                  <button class="primary" onclick={onEnter}>Enter the console</button>
                  <button onclick={() => scrollTo('drift')}>See how it works <span aria-hidden="true">↓</span></button>
                </div>
              </div>
            </div>
          {:else if s.id === 'drift'}
            <div class="stage">
              <div class="copy">
                <p class="eyebrow warn"><b aria-hidden="true">▌</b> The challenge</p>
                <Headline lines={[['Machines set up'], ['by hand'], ['drift apart.', 'warn']]} />
                <p class="lede">
                  A version pinned here, a tweak forgotten there, an update that broke at two in the morning and cannot be undone. Every box ends up subtly different, and
                  nobody can say exactly why.
                </p>
                <ul class="drift-list" aria-label="Examples of configuration drift">
                  {#each DRIFT as [pkg, a, b] (pkg)}
                    <li><span class="pkg">{pkg}</span><span class="was">{a}</span><span class="vs">≠</span><span class="bad">{b}</span></li>
                  {/each}
                </ul>
              </div>
            </div>
          {:else if s.id === 'assemble'}
            <div class="stage">
              <div class="copy">
                <p class="eyebrow"><b aria-hidden="true">▌</b> The NixOS way</p>
                <Headline lines={[['Declare it once.'], ['Build it the same,', 'acc'], ['everywhere.', 'acc']]} />
                <p class="lede">
                  Every package is built from a pure description into <b>/nix/store/&lt;hash&gt;-name</b>. The same input gives the same result, on every machine, every
                  time.
                </p>
                <div class="counter" aria-live="off">
                  <p class="muted">From 100&nbsp;000+ packages in nixpkgs</p>
                  <div class="num">
                    <span class="sr-only">{formatGrouped(countdown(counterT(i)))}</span>
                    <pre class="big acc" aria-hidden="true" use:bigText={formatGrouped(countdown(counterT(i))).replace(/\s/g, ' ')}></pre>
                  </div>
                  <p class="unit">{counterLabel(counterT(i))}</p>
                </div>
              </div>
            </div>
          {:else if s.id === 'planet'}
            <div class="stage">
              <div class="copy">
                <Headline lines={[['Every package'], ['is a node.'], ['Every machine,', 'acc'], ['a closure.', 'acc']]} />
                <p class="lede">
                  Dependencies are edges in one graph. A machine runs exactly the closure its description reaches, hash by hash, so nothing is shared by accident and
                  nothing is missing by surprise.
                </p>
              </div>
            </div>
          {:else if s.id === 'orbit'}
            <div class="stage">
              <div class="copy">
                <p class="eyebrow"><b aria-hidden="true">▌</b> The fleet</p>
                <Headline lines={[['Your machines,'], ['in orbit.', 'acc']]} />
                <p class="lede">
                  nixos-manager evaluates every machine from the flake and shows the fleet as a living map: what each one runs, where it comes from, and why.
                </p>
                <ul class="fleet" aria-label="The example fleet">
                  {#each FLEET as [name, arch, roles] (name)}
                    <li><b class="name"><span aria-hidden="true">▣&nbsp;</span>{name}</b><span class="arch">{arch}</span><span class="roles">{roles}</span></li>
                  {/each}
                </ul>
              </div>
            </div>
          {:else if s.id === 'blueprint'}
            <div class="stage bp-stage">
              <Blueprint t={scrub(i)} />
              <div class="bp-head">
                <p class="eyebrow"><b aria-hidden="true">▌</b> The blueprint</p>
                <h2 class="statement">Drawn, connected, checked.</h2>
                <p class="bp-sub">The configuration graph of the fleet, evaluated node by node.</p>
              </div>
            </div>
          {:else if s.id === 'console'}
            <div class="pin">
              <div class="stage console-stage">
                <div class="copy">
                  <p class="eyebrow"><b aria-hidden="true">▌</b> The console</p>
                  <Headline lines={[['Visual.'], ['Reliable.'], ['Yours.', 'acc']]} />
                  <p class="lede">
                    Change packages and options by clicking. Every edit lands in a per-machine <b>managed.json</b>; every change is checked on exactly the staged content
                    before it can be committed, and again before it can be pushed.
                  </p>
                </div>
                <Showcase />
              </div>
            </div>
            <div class="tail">
              <ul class="wall">
                {#each FEATURES as [glyph, name, line] (name)}
                  <li>
                    <h3><span class="acc" aria-hidden="true">{glyph}</span> {name}</h3>
                    <p>{line}</p>
                  </li>
                {/each}
              </ul>
              <div class="rule"></div>
              <dl class="stats">
                {#each STATS as [n, label] (label)}
                  <div>
                    <dt><span class="sr-only">{n}</span><pre class="big" aria-hidden="true" use:bigText={n}></pre></dt>
                    <dd>{label}</dd>
                  </div>
                {/each}
              </dl>
            </div>
          {:else if s.id === 'deploy'}
            <div class="stage">
              <div class="copy">
                <p class="eyebrow"><b aria-hidden="true">▌</b> The round trip</p>
                <Headline lines={[['A change,'], ['from click', 'acc'], ['to switch.', 'acc']]} />
                <p class="lede">Commits land on your own Forgejo. Each machine pulls with comin and switches itself. Nothing reaches a machine that has not passed the checks.</p>
                <DeployRail t={scrub(i)} />
              </div>
            </div>
          {:else}
            <div class="pin">
              <div class="stage outro-stage">
                <div class="copy mid">
                  <Headline lines={[['Your fleet,'], ['declared.', 'acc']]} />
                  <p class="lede">One repository. Every machine evaluated. No unchecked push. The console is a login away.</p>
                  <div class="ctas">
                    {#if back}
                      <button class="primary" onclick={back}>Back to the console</button>
                    {:else}
                      <button class="primary" onclick={onEnter}>Enter the console</button>
                    {/if}
                  </div>
                </div>
                <footer class="footer">
                  <div class="cols">
                    <div>
                      <p class="k">nixos-manager</p>
                      <p>Self-hosted. One flake. Every machine.</p>
                    </div>
                    <div>
                      <p class="k">Built on</p>
                      <p>NixOS · nixpkgs · flakes</p>
                      <p>Forgejo · comin</p>
                    </div>
                    <div>
                      <p class="k">Checks</p>
                      <p>nix flake check</p>
                      <p>a build of every changed machine</p>
                    </div>
                    <div>
                      <p class="k">Access</p>
                      <p>Web console · MCP for agents</p>
                      <p>Your LAN or Tailscale, nothing else</p>
                    </div>
                  </div>
                  <div class="rule"></div>
                  <p class="fine">Store hashes shown are illustrative. Package counts: nixpkgs, 2025.</p>
                </footer>
              </div>
            </div>
          {/if}
        </section>
      {/each}
    </main>

    <div class="foot" aria-hidden="true">
      <span class="meta">fleet · 3 machines · 1 flake</span>
      <span class="spacer"></span>
      <span class="meta">{progress}</span>
      {#if pos.index === 0}<span>scroll ↓</span>{/if}
      <span class="meta">/nix/store/8f3k…-nixos-system-homeserver-26.05</span>
    </div>
  </div>
</div>

<style>
  .intro {
    position: fixed;
    inset: 0;
    z-index: 40;
    overflow: hidden auto;
    overscroll-behavior: contain;
    color: var(--fg);
    background: var(--bg);
    outline: 0;
  }
  /* The story itself takes focus so the keys scroll it; it is not a control to light up. */
  .intro:focus-visible {
    color: var(--fg);
    background: var(--bg);
  }
  .content {
    position: relative;
  }
  main {
    position: relative;
    z-index: 1;
  }

  /* ---- the bars ---- */
  .nav,
  .foot {
    position: fixed;
    right: 0;
    left: 0;
    z-index: 6;
    display: flex;
    gap: 3ch;
    align-items: baseline;
    padding: var(--lh) 3ch;
    white-space: nowrap;
    background: var(--bg);
  }
  .nav {
    top: 0;
  }
  .foot {
    bottom: 0;
    color: var(--fg-faint);
  }
  .brand {
    color: var(--fg-strong);
    font-weight: 700;
  }
  .links {
    display: flex;
    gap: 3ch;
  }
  .links a {
    color: var(--fg-muted);
  }
  .links a.on {
    color: var(--fg-strong);
    font-weight: 700;
  }
  .links a:hover,
  .links a:focus-visible {
    color: var(--on-accent);
  }

  /* ---- chapters ---- */
  .chapter {
    position: relative;
    height: calc(var(--len) * 100vh);
  }
  .chapter.flow {
    height: auto;
  }
  .pin {
    height: calc(var(--len) * 100vh);
  }
  .stage {
    position: sticky;
    top: 0;
    height: 100vh;
    padding: calc(var(--lh) * 8) 6ch 0;
    overflow: hidden;
  }
  .copy {
    position: relative;
    z-index: 1;
    width: 78ch;
    max-width: 100%;
  }
  .copy > :global(* + *) {
    margin-top: var(--lh);
  }
  .copy > :global(.headline) {
    margin-top: 0;
  }
  .eyebrow {
    margin-bottom: var(--lh);
    font-weight: 700;
  }
  .eyebrow b {
    color: var(--accent);
  }
  .eyebrow.warn b {
    color: var(--yellow);
  }
  .lede {
    max-width: 62ch;
  }
  .ctas {
    display: flex;
    flex-wrap: wrap;
    gap: 2ch;
  }

  /* ---- drift ---- */
  .drift-list li {
    display: grid;
    grid-template-columns: 12ch 9ch 4ch auto;
  }
  .pkg {
    color: var(--fg-muted);
  }
  .was {
    color: var(--fg);
  }
  .vs,
  .bad {
    color: var(--yellow);
  }

  /* A list or a figure under the lede stands two rows off it. */
  .copy > :is(.drift-list, .fleet, .counter, :global(.rail)) {
    margin-top: calc(var(--lh) * 2);
  }

  /* ---- assemble: the counter ---- */
  .unit {
    color: var(--accent);
    font-weight: 700;
  }

  /* ---- orbit ---- */
  .fleet li {
    display: grid;
    grid-template-columns: 15ch auto;
  }
  .fleet li + li {
    margin-top: var(--lh);
  }
  .arch,
  .roles {
    color: var(--accent);
  }
  .roles {
    grid-column: 1 / -1;
    padding-left: 2ch;
  }

  /* ---- blueprint ---- */
  .bp-stage {
    padding-top: calc(var(--lh) * 4);
    background: var(--bg);
  }
  .bp-head {
    position: relative;
    z-index: 1;
    width: 64ch;
  }
  .statement {
    text-transform: none;
  }
  .statement::before {
    content: none;
  }
  .bp-sub {
    color: var(--fg-muted);
  }

  /* ---- console ---- */
  .console-stage {
    display: flex;
    gap: 4ch;
    justify-content: space-between;
    padding-top: calc(var(--lh) * 7);
  }
  .console-stage .copy {
    padding-top: var(--lh);
  }
  .tail {
    padding: calc(var(--lh) * 4) 6ch calc(var(--lh) * 6);
  }
  .wall {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: calc(var(--lh) * 2) 4ch;
    margin-bottom: var(--lh);
  }
  .wall p {
    max-width: 44ch;
    color: var(--fg-muted);
  }
  .stats {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: 4ch;
    margin-top: var(--lh);
  }
  .stats dd {
    color: var(--fg-muted);
  }

  /* ---- outro ---- */
  .outro-stage {
    display: grid;
    grid-template-rows: 1fr auto;
    padding-top: calc(var(--lh) * 9);
  }
  .copy.mid {
    display: grid;
    align-content: start;
    justify-items: center;
    width: auto;
    text-align: center;
  }
  /* The planet sets behind the footer like a horizon. */
  .footer {
    position: relative;
    z-index: 1;
    margin: 0 -6ch;
    padding: var(--lh) 6ch calc(var(--lh) * 4);
    color: var(--fg-muted);
    background: var(--bg);
  }
  .footer .cols {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: 4ch;
  }
  .footer .k {
    color: var(--fg-strong);
    font-weight: 700;
  }
  .footer .fine {
    color: var(--fg-faint);
  }

  /* ---- reduced motion: no pinning, no scrubbing, everything readable ---- */
  .rm .chapter,
  .rm .pin {
    height: auto;
    min-height: 100vh;
  }
  .rm .stage {
    position: relative;
    height: auto;
    min-height: 100vh;
    padding-bottom: calc(var(--lh) * 4);
  }

  /* Below 1200px the shots no longer fit beside the copy: they flow under it. */
  @media (max-width: 1200px) {
    .chapter.console .pin {
      height: auto;
    }
    .console-stage {
      position: relative;
      flex-direction: column;
      height: auto;
    }
  }
  @media (max-width: 860px) {
    .links,
    .meta {
      display: none;
    }
    .stage,
    .tail {
      padding-right: 2ch;
      padding-left: 2ch;
    }
    .wall,
    .stats,
    .footer .cols {
      grid-template-columns: minmax(0, 1fr);
    }
    .footer {
      margin: 0 -2ch;
      padding-right: 2ch;
      padding-left: 2ch;
    }
  }
</style>
