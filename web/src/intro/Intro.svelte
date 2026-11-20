<script lang="ts">
  // The intro: a scroll story in nine chapters over one WebGL scene. This file owns the scrolling,
  // the copy and the layout; Scene.svelte owns the canvas, Blueprint.svelte the drawing.
  import { onMount } from 'svelte'
  import './intro.css'
  import Blueprint from './blueprint/Blueprint.svelte'
  import DeployRail from './DeployRail.svelte'
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
  let narrow = $state(innerWidth < 860)
  let lenis: { scrollTo(target: number | HTMLElement, opts?: { offset?: number }): void; destroy(): void } | undefined

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
    narrow = innerWidth < 860
    sections = STORY.map((s) => ({ id: s.id, top: els[s.id]!.offsetTop, height: els[s.id]!.offsetHeight, pinned: s.len > 0 }))
    update()
  }
  let frame = 0
  function onScroll() {
    if (frame) return
    frame = requestAnimationFrame(() => {
      frame = 0
      update()
    })
  }
  function update() {
    if (!sections.length) return
    const p = locate(root.scrollTop, root.clientHeight, sections)
    if (p.id !== pos.id || p.t !== pos.t) pos = p
  }
  const progressOf = (i: number) => (pos.index === i ? pos.t : pos.index > i ? 1 : 0)
  // With reduced motion nothing scrubs: the counter, the drawing and the rail show their finished state.
  const scrub = (i: number) => (reducedMotion ? 1 : progressOf(i))
  // The counter runs over the second half of the assemble chapter, after the statement has faded.
  const counterT = (i: number) => (scrub(i) - 0.55) / 0.4

  function scrollTo(id: StoryChapter) {
    const el = els[id]
    if (!el) return
    if (lenis) lenis.scrollTo(el.offsetTop)
    else root.scrollTo({ top: el.offsetTop, behavior: reducedMotion ? 'auto' : 'smooth' })
  }

  onMount(() => {
    measure()
    root.focus({ preventScroll: true })
    const ro = new ResizeObserver(measure)
    ro.observe(root.firstElementChild as Element)
    if (!reducedMotion) {
      import('lenis').then(({ default: Lenis }) => {
        lenis = new Lenis({ wrapper: root, content: root.firstElementChild as HTMLElement, autoRaf: true, lerp: 0.085, wheelMultiplier: 0.9 })
      })
    }
    return () => {
      ro.disconnect()
      lenis?.destroy()
      cancelAnimationFrame(frame)
    }
  })

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
</script>

<div class="intro" class:rm={reducedMotion} bind:this={root} onscroll={onScroll} tabindex="-1">
  <div class="content">
    <Scene chapter={sceneChapter} t={sceneT} visible={sceneVisible} {reducedMotion} machines={MACHINES} />

    <header class="nav" class:dim={pos.index > 0 && pos.index < 8}>
      <a class="brand" href="#top" onclick={(e) => (e.preventDefault(), scrollTo('hero'))} aria-label="nixos-manager, back to the top">
        <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <path d="M12 2v20M3.4 7l17.2 10M3.4 17L20.6 7" />
          <path d="M9 4.2 12 2l3 2.2M9 19.8 12 22l3-2.2M3.6 10.6 3.4 7l3.3-1.3M17.1 5.7 20.6 7l-.2 3.6M3.6 13.4 3.4 17l3.3 1.3M17.1 18.3 20.6 17l-.2-3.6" />
        </svg>
        <span>nixos-manager</span>
      </a>
      <nav class="links" aria-label="Chapters">
        {#each nav as [id, label] (id)}
          <a href="#{id}" onclick={(e) => (e.preventDefault(), scrollTo(id))}>{label}</a>
        {/each}
      </nav>
      {#if back}
        <button class="btn small" onclick={back}>Back to the console</button>
      {:else}
        <button class="btn small" onclick={onEnter}>Enter the console</button>
      {/if}
    </header>

    <main>
      <!-- 1 · hero -->
      {#each STORY as s, i (s.id)}
        <section
          class="chapter {s.id}"
          class:flow={s.id === 'console' || s.id === 'outro'}
          id={s.id}
          bind:this={els[s.id]}
          style:--len={s.len || 1}
          style:--p={progressOf(i)}
        >
          {#if s.id === 'hero'}
            <div class="stage">
              <div class="inner hero-inner">
                <h1 class="display halo">
                  <span class="clip"><span class="rise" style:--i="0">Every machine.</span></span>
                  <span class="clip"><span class="rise" style:--i="1">One source</span></span>
                  <span class="clip"><span class="rise" style:--i="2">of truth.</span></span>
                </h1>
                <p class="lede halo center rise" style:--i="3">A self-hosted console for every NixOS machine you own: evaluated from one flake, drawn as one graph, checked before anything ships.</p>
                <div class="ctas rise" style:--i="4">
                  <button class="btn solid" onclick={onEnter}>Enter the console</button>
                  <button class="btn" onclick={() => scrollTo('drift')}>See how it works</button>
                </div>
                <div class="hero-meta mono" aria-hidden="true">
                  <span>fleet · 3 machines · 1 flake</span>
                  <span class="cue"><span class="cue-line"></span>scroll</span>
                  <span>/nix/store/8f3k…-nixos-system-homeserver-26.05</span>
                </div>
              </div>
            </div>
          {:else if s.id === 'drift'}
            <div class="stage">
              <div class="inner col">
                <p class="eyebrow warm rv" style:--a="0.02" style:--b="0.1">The challenge</p>
                <h2 class="statement halo">
                  <span class="clip"><span style:--a="0.06" style:--b="0.22">Machines set up by hand</span></span>
                  <span class="clip"><span style:--a="0.12" style:--b="0.3">drift apart.</span></span>
                </h2>
                <p class="lede halo rv" style:--a="0.28" style:--b="0.42">
                  A version pinned here, a tweak forgotten there, an update that broke at two in the morning and cannot be undone. Every box ends up subtly different, and
                  nobody can say exactly why.
                </p>
                <ul class="drift-list mono" aria-label="Examples of configuration drift">
                  {#each DRIFT as [pkg, a, b], j (pkg)}
                    <li class="rv" style:--a={0.5 + j * 0.08} style:--b={0.62 + j * 0.08}>
                      <span class="pkg">{pkg}</span><span class="ok">{a}</span><span class="vs">≠</span><span class="bad">{b}</span>
                    </li>
                  {/each}
                </ul>
              </div>
            </div>
          {:else if s.id === 'assemble'}
            <div class="stage">
              <div class="inner col">
                <div class="out" style:--c="0.42" style:--d="0.52">
                  <p class="eyebrow rv" style:--a="0.02" style:--b="0.1">The NixOS way</p>
                  <h2 class="statement halo">
                    <span class="clip"><span style:--a="0.06" style:--b="0.2">Declare it once.</span></span>
                    <span class="clip"><span style:--a="0.12" style:--b="0.28">Build it the same, everywhere.</span></span>
                  </h2>
                  <p class="lede halo rv" style:--a="0.24" style:--b="0.36">
                    Every package is built from a pure description into <b class="mono">/nix/store/&lt;hash&gt;-name</b>. The same input gives the same result, on every
                    machine, every time.
                  </p>
                </div>
                <div class="counter halo rv" style:--a="0.5" style:--b="0.58" aria-live="off">
                  <p class="eyebrow">From 100&#8239;000+ packages in nixpkgs</p>
                  <p class="big num">{formatGrouped(countdown(counterT(i)))}</p>
                  <p class="unit mono">{counterLabel(counterT(i))}</p>
                </div>
              </div>
            </div>
          {:else if s.id === 'planet'}
            <div class="stage">
              <div class="inner left plate">
                <h2 class="statement halo">
                  <span class="clip"><span style:--a="0.08" style:--b="0.24">Every package is a node.</span></span>
                  <span class="clip"><span style:--a="0.16" style:--b="0.32">Every machine, a closure.</span></span>
                </h2>
                <p class="lede halo rv" style:--a="0.3" style:--b="0.44">
                  Dependencies are edges in one graph. A machine runs exactly the closure its description reaches, hash by hash, so nothing is shared by accident and
                  nothing is missing by surprise.
                </p>
              </div>
            </div>
          {:else if s.id === 'orbit'}
            <div class="stage">
              <div class="inner left plate">
                <p class="eyebrow rv" style:--a="0.02" style:--b="0.1">The fleet</p>
                <h2 class="statement halo">
                  <span class="clip"><span style:--a="0.06" style:--b="0.22">Your machines,</span></span>
                  <span class="clip"><span style:--a="0.12" style:--b="0.28">in orbit.</span></span>
                </h2>
                <p class="lede halo rv" style:--a="0.26" style:--b="0.4">
                  nixos-manager evaluates every machine from the flake and shows the fleet as a living map: what each one runs, where it comes from, and why.
                </p>
                <ul class="fleet mono" aria-label="The example fleet">
                  {#each FLEET as [name, arch, roles], j (name)}
                    <li class="rv" style:--a={0.46 + j * 0.1} style:--b={0.58 + j * 0.1}>
                      <span class="name">{name}</span><span class="arch">{arch}</span><span class="roles">{roles}</span>
                    </li>
                  {/each}
                </ul>
              </div>
            </div>
          {:else if s.id === 'blueprint'}
            <div class="stage bp-stage">
              <div class="bp-head">
                <p class="eyebrow">The blueprint</p>
                <h2 class="statement">Drawn, connected, checked.</h2>
                <p class="bp-sub">The configuration graph of the fleet, evaluated node by node.</p>
              </div>
              <div class="bp-frame">
                <Blueprint t={scrub(i)} {narrow} />
              </div>
            </div>
          {:else if s.id === 'console'}
            <div class="pin">
              <div class="stage">
                <div class="inner console-head">
                  <p class="eyebrow rv" style:--a="0" style:--b="0.03">The console</p>
                  <h2 class="display sm">
                    <span class="clip"><span style:--a="0.01" style:--b="0.05">Visual.</span></span>
                    <span class="clip"><span style:--a="0.025" style:--b="0.065">Reliable.</span></span>
                    <span class="clip"><span style:--a="0.04" style:--b="0.08">Yours.</span></span>
                  </h2>
                  <p class="lede rv" style:--a="0.06" style:--b="0.1">
                    Change packages and options by clicking. Every edit lands in a per-machine <b class="mono">managed.json</b>; every change is checked on exactly the
                    staged content before it can be committed, and again before it can be pushed.
                  </p>
                </div>
              </div>
            </div>
            <div class="inner wide">
              <Showcase t={progressOf(i)} />
            </div>
          {:else if s.id === 'deploy'}
            <div class="stage">
              <div class="inner deploy-inner">
                <div class="left plate">
                  <p class="eyebrow rv" style:--a="0.02" style:--b="0.1">The round trip</p>
                  <h2 class="statement halo">
                    <span class="clip"><span style:--a="0.04" style:--b="0.18">A change,</span></span>
                    <span class="clip"><span style:--a="0.1" style:--b="0.24">from click to switch.</span></span>
                  </h2>
                  <p class="lede halo rv" style:--a="0.2" style:--b="0.32">
                    Commits land on your own Forgejo. Each machine pulls with comin and switches itself. Nothing reaches a machine that has not passed the checks.
                  </p>
                </div>
                <div class="rail-wrap halo">
                  <DeployRail t={scrub(i)} />
                </div>
              </div>
            </div>
          {:else}
            <div class="pin">
              <div class="stage">
                <div class="inner col outro-inner">
                  <h2 class="display halo">
                    <span class="clip"><span style:--a="0.04" style:--b="0.22">Your fleet,</span></span>
                    <span class="clip"><span style:--a="0.1" style:--b="0.28">declared.</span></span>
                  </h2>
                  <p class="lede halo center rv" style:--a="0.22" style:--b="0.36">One repository. Every machine evaluated. No unchecked push. The console is a login away.</p>
                  <div class="ctas rv" style:--a="0.28" style:--b="0.42">
                    {#if back}
                      <button class="btn solid" onclick={back}>Back to the console</button>
                    {:else}
                      <button class="btn solid" onclick={onEnter}>Enter the console</button>
                    {/if}
                  </div>
                </div>
                <footer class="footer">
                <div class="cols">
                  <div>
                    <p class="mono k">nixos-manager</p>
                    <p>Self-hosted. One flake. Every machine.</p>
                  </div>
                  <div>
                    <p class="mono k">Built on</p>
                    <p>NixOS · nixpkgs · flakes</p>
                    <p>Forgejo · comin</p>
                  </div>
                  <div>
                    <p class="mono k">Checks</p>
                    <p>nix flake check</p>
                    <p>a build of every changed machine</p>
                  </div>
                  <div>
                    <p class="mono k">Access</p>
                    <p>Web console · MCP for agents</p>
                    <p>Your LAN or Tailscale, nothing else</p>
                  </div>
                </div>
                <div class="rule"></div>
                <p class="mono fine">Store hashes shown are illustrative. Package counts: nixpkgs, 2025.</p>
                </footer>
              </div>
            </div>
          {/if}
        </section>
      {/each}
    </main>
  </div>
</div>

<style>
  .content {
    position: relative;
  }
  main {
    position: relative;
    z-index: 1;
  }

  /* ---- nav ---- */
  .nav {
    position: fixed;
    top: 0;
    right: 0;
    left: 0;
    z-index: 5;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
    padding: 18px var(--gutter);
    transition: opacity 400ms var(--i-ease);
  }
  .nav.dim {
    opacity: 0.72;
  }
  .nav:hover,
  .nav:focus-within {
    opacity: 1;
  }
  .brand {
    display: inline-flex;
    align-items: center;
    gap: 10px;
    color: var(--i-fg);
    font-weight: 600;
    font-size: 0.95rem;
    letter-spacing: -0.01em;
    text-decoration: none;
  }
  .brand svg {
    color: var(--i-blue);
  }
  .links {
    display: flex;
    gap: clamp(16px, 2.4vw, 36px);
  }
  .links a {
    color: var(--i-muted);
    font-size: 0.86rem;
    text-decoration: none;
    transition: color 200ms var(--i-ease);
  }
  .links a:hover {
    color: var(--i-fg);
  }

  /* ---- shared layout ---- */
  .inner.col {
    justify-items: center;
    text-align: center;
  }
  .inner.col .lede {
    text-align: center;
  }
  .inner.left {
    justify-items: start;
    max-width: 760px;
  }
  /* A soft dark plate under text that sits over the scene, so it reads whatever the planet is doing. */
  .plate {
    position: relative;
    isolation: isolate;
  }
  .plate::before {
    content: '';
    position: absolute;
    inset: -20% -30% -20% -40%;
    z-index: -1;
    background: radial-gradient(ellipse at 40% 50%, rgb(5 7 10 / 0.88), rgb(5 7 10 / 0.55) 45%, transparent 72%);
    pointer-events: none;
  }
  .deploy-inner .plate::before {
    inset: -30% -30% -30% -50%;
  }
  .inner.wide {
    max-width: 1320px;
    margin: 0 auto;
    padding-top: clamp(100px, 16vh, 180px);
    padding-bottom: clamp(100px, 16vh, 180px);
  }
  .eyebrow {
    margin-bottom: 28px;
  }
  .statement + .lede,
  .display + .lede {
    margin-top: 28px;
  }
  .ctas {
    display: flex;
    flex-wrap: wrap;
    gap: 12px;
    margin-top: 40px;
  }

  /* ---- hero ---- */
  .hero-inner {
    justify-items: center;
    text-align: center;
  }
  /* The hero is the one authored entrance: lines rise once on load, before the scene has even loaded. */
  .rise {
    animation: rise 1.1s var(--i-ease) both;
    animation-delay: calc(0.08s + var(--i) * 0.11s);
  }
  @keyframes rise {
    from {
      opacity: 0;
      transform: translate3d(0, 0.9em, 0);
    }
    to {
      opacity: 1;
      transform: none;
    }
  }
  .hero-inner .clip .rise {
    opacity: 1;
    animation-name: rise-clip;
  }
  @keyframes rise-clip {
    from {
      transform: translate3d(0, 112%, 0);
    }
    to {
      transform: none;
    }
  }
  :global(.intro.rm) .rise {
    animation: none;
  }
  .hero-inner .lede {
    max-width: 52ch;
  }
  .hero-meta {
    position: absolute;
    right: var(--gutter);
    bottom: 26px;
    left: var(--gutter);
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    color: var(--i-muted);
    font-size: 0.7rem;
    letter-spacing: 0.08em;
  }
  .cue {
    display: inline-flex;
    flex-direction: column;
    align-items: center;
    gap: 10px;
    text-transform: uppercase;
  }
  .cue-line {
    width: 1px;
    height: 44px;
    background: linear-gradient(var(--i-blue), transparent);
    animation: cue 2.4s var(--i-ease) infinite;
    transform-origin: top;
  }
  @keyframes cue {
    0% {
      transform: scaleY(0);
      opacity: 0;
    }
    35% {
      transform: scaleY(1);
      opacity: 1;
    }
    100% {
      transform: scaleY(1) translateY(44px);
      opacity: 0;
    }
  }
  :global(.intro.rm) .cue-line {
    animation: none;
  }

  /* ---- drift ---- */
  .drift-list {
    display: grid;
    gap: 10px;
    margin: 44px 0 0;
    padding: 0;
    list-style: none;
    font-size: 0.82rem;
  }
  .drift-list li {
    display: flex;
    gap: 18px;
    justify-content: center;
  }
  .drift-list .pkg {
    min-width: 7ch;
    color: var(--i-muted);
    text-align: right;
  }
  .drift-list .ok {
    color: var(--i-fg);
  }
  .drift-list .vs {
    color: var(--i-orange);
  }
  .drift-list .bad {
    color: var(--i-orange);
  }

  /* ---- assemble: the counter ---- */
  .counter {
    position: absolute;
    inset: 0;
    display: grid;
    place-content: center;
    gap: 18px;
    text-align: center;
    pointer-events: none;
  }
  /* Reduced motion shows every state at once: stack the counter under the statement, not over it. */
  .intro.rm .counter {
    position: static;
    margin-top: 56px;
  }
  .counter .eyebrow {
    justify-content: center;
    margin: 0;
  }
  .counter .eyebrow::before {
    display: none;
  }
  .big {
    font-weight: 800;
    font-size: clamp(5rem, 17vw, 17rem);
    line-height: 0.9;
    letter-spacing: -0.05em;
  }
  .unit {
    color: var(--i-blue);
    font-size: 0.8rem;
    letter-spacing: 0.22em;
    text-transform: uppercase;
  }

  /* ---- orbit ---- */
  .fleet {
    display: grid;
    gap: 12px;
    margin: 44px 0 0;
    padding: 0;
    list-style: none;
    font-size: 0.82rem;
  }
  .fleet li {
    display: grid;
    grid-template-columns: 12ch 13ch 1fr;
    gap: 16px;
    padding-top: 10px;
    border-top: 1px solid var(--i-line);
  }
  .fleet .name {
    color: var(--i-fg);
    font-weight: 600;
  }
  .fleet .arch {
    color: var(--i-blue);
  }
  .fleet .roles {
    color: var(--i-muted);
  }

  /* ---- blueprint ---- */
  .bp-stage {
    grid-template-rows: auto 1fr;
    padding: 84px var(--gutter) 24px;
    background: var(--i-bg);
  }
  .bp-head {
    display: grid;
    grid-template-columns: auto 1fr;
    column-gap: 40px;
    align-items: baseline;
    padding-bottom: 16px;
  }
  .bp-head .eyebrow {
    grid-column: 1 / -1;
    margin: 0 0 10px;
  }
  .bp-head .statement {
    font-size: clamp(1.5rem, 2.4vw, 2.2rem);
  }
  .bp-sub {
    color: var(--i-muted);
    font-size: 0.95rem;
  }
  .bp-frame {
    min-height: 0;
  }

  /* ---- pinned head + flowing tail (console, outro) ---- */
  .pin {
    height: calc(var(--len) * 100vh);
  }
  :global(.intro.rm) .pin {
    height: auto;
  }

  /* ---- console ---- */
  .console-head {
    justify-items: start;
    max-width: 960px;
  }
  .console-head .lede {
    margin-top: 32px;
  }
  .console .inner.wide {
    padding-top: 0;
  }

  /* ---- deploy ---- */
  .deploy-inner {
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
    gap: clamp(32px, 6vw, 96px);
    align-items: center;
  }
  .rail-wrap {
    position: relative;
    max-width: 480px;
    justify-self: end;
    width: 100%;
    isolation: isolate;
  }
  .rail-wrap::before {
    content: '';
    position: absolute;
    inset: -14% -20% -14% -16%;
    z-index: -1;
    background: radial-gradient(ellipse at 50% 50%, rgb(5 7 10 / 0.92), rgb(5 7 10 / 0.7) 50%, transparent 76%);
    pointer-events: none;
  }

  /* ---- outro ---- */
  /* The closing stage is the final viewport: statement centred above, footer anchored below. */
  .outro .stage {
    grid-template-rows: 1fr auto;
  }
  .outro-inner {
    min-height: 0;
  }
  .footer {
    position: relative;
    z-index: 1;
    display: grid;
    gap: 24px;
    padding: 56px var(--gutter) 28px;
    color: var(--i-muted);
    font-size: 0.88rem;
    background: linear-gradient(transparent, rgb(5 7 10 / 0.94) 40px, var(--i-bg));
  }
  .footer .cols {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: 24px;
  }
  .footer .cols div {
    display: grid;
    align-content: start;
    gap: 6px;
  }
  .footer .k {
    margin-bottom: 8px;
    color: var(--i-fg);
    font-size: 0.7rem;
    letter-spacing: 0.14em;
    text-transform: uppercase;
  }
  .footer .fine {
    font-size: 0.7rem;
  }

  @media (max-width: 860px) {
    .links {
      display: none;
    }
    .nav {
      padding: 14px var(--gutter);
    }
    .hero-meta span:last-child,
    .hero-meta span:first-child {
      display: none;
    }
    .hero-meta {
      justify-content: center;
    }
    .deploy-inner {
      grid-template-columns: 1fr;
      align-content: start;
      gap: 28px;
    }
    .rail-wrap {
      justify-self: stretch;
    }
    .fleet li {
      grid-template-columns: 1fr;
      gap: 2px;
    }
    .bp-stage {
      padding: 76px 12px 16px;
    }
    .bp-head {
      grid-template-columns: 1fr;
      padding: 0 8px 10px;
    }
    .bp-sub {
      display: none;
    }
    .footer {
      gap: 16px;
      padding-top: 32px;
      font-size: 0.8rem;
    }
    .footer .cols {
      grid-template-columns: 1fr 1fr;
      gap: 14px;
    }
    .footer .cols div {
      gap: 2px;
    }
    .footer .k {
      margin-bottom: 4px;
    }
    .drift-list li {
      flex-wrap: wrap;
      gap: 8px 12px;
    }
  }
</style>
