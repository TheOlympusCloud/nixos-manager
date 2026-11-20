// Repository actions shared by the top bar, History, the command palette and Changes.
import PushImpact from '../components/PushImpact.svelte'
import { ApiError, api, type MachineDeployment } from './api'
import { rollbackOf } from './deployments'
import { loadDeployments } from './deployments.svelte'
import { go } from './router.svelte'
import { app, checkStaged, fileTabs, refresh, syncOpenTabs } from './state.svelte'
import { alertBox, confirmBox, report, toast } from './ui.svelte'

/**
 * The commit message being written in Changes; survives leaving the view, and merge or revert can prefill it.
 * pushTree: the staged tree of a rollback; committing exactly it goes on to the push dialog.
 */
export const draft = $state({ message: '', pushTree: '' })

export const repo = $state({ busy: '' as '' | 'pull' | 'push' | 'revert' | 'merge' | 'switch' | 'branch' | 'try' | 'end' })

/** Why Pull and Push cannot work, or '' when they can. */
export const noRemote = () => (app.info && !app.info.remote ? 'No remote is configured: connect the Forgejo repository in Settings.' : '')

const message = (e: unknown) => (e instanceof Error ? e.message : String(e))

export async function pullOrPush(kind: 'pull' | 'push') {
  if (repo.busy) return
  const why = noRemote()
  if (why) return toast(why, 'info', kind === 'pull' ? 'Cannot pull' : 'Cannot push', { label: 'Open Settings', run: () => go('#/settings') })
  const st = app.status
  // A push deploys: first show what it changes on each machine, unless it has nothing new to send.
  if (kind === 'push' && !(st?.upstream && st.ahead === 0)) {
    const ok = await confirmBox({
      title: `Push ${st?.branch || 'this branch'}?`,
      message: 'The machines deploy what is pushed. Each one gets this, compared with what is pushed now.',
      body: PushImpact,
      wide: true,
      ok: 'Push',
    })
    if (!ok || repo.busy) return
  }
  repo.busy = kind
  try {
    let out: string
    try {
      out = await api[kind]()
    } catch (e) {
      // 412: the commit to push was never checked as a whole, or not against what is pushed now; the server says which.
      if (!(kind === 'push' && e instanceof ApiError && e.status === 412)) throw e
      const why = e.message.charAt(0).toUpperCase() + e.message.slice(1)
      const force = await confirmBox({
        title: 'Push an unchecked commit?',
        message: `${why}. Pushed anyway, it deploys unchecked: a machine it changes may fail to build it.`,
        ok: 'Push unchecked',
        danger: true,
      })
      if (!force) return
      out = await api.push(true)
    }
    toast(out.trim().split('\n').filter(Boolean).slice(-2).join('\n') || (kind === 'pull' ? 'Already up to date.' : 'Pushed.'), 'ok', kind === 'pull' ? 'Pulled' : 'Pushed')
  } catch (e) {
    if (!(e instanceof ApiError && e.status === 401)) await alertBox({ title: kind === 'pull' ? 'Pull failed' : 'Push failed', pre: message(e) })
  } finally {
    repo.busy = ''
    await Promise.all([refresh(), kind === 'pull' && syncOpenTabs()])
    app.logVersion++
  }
}

const dirtyTabs = () => fileTabs().filter((t) => t.dirty)

/** Stage the inverse of a commit, then send the user to Changes with a message ready. */
export async function revertCommit(sha: string, subject: string) {
  const short = sha.slice(0, 7)
  const ok = await confirmBox({
    title: 'Revert this commit?',
    message: `This stages the opposite of ${short} “${subject}” in the working copy. Nothing is committed: review it in Changes, run the checks, then commit.`,
    ok: 'Stage the revert',
  })
  if (!ok) return
  repo.busy = 'revert'
  try {
    await api.revert(sha)
    draft.message = `Revert "${subject}"\n\nThis reverts commit ${sha}.`
    toast(`The revert of ${short} is staged. Check and commit it here.`, 'ok', 'Revert staged')
    go('#/changes')
  } catch (e) {
    if (e instanceof ApiError && e.status === 409) await alertBox({ title: 'Cannot revert', ...conflictText(e.message, 'revert') })
    else report(e, 'Revert failed')
  } finally {
    repo.busy = ''
    await Promise.all([refresh(), syncOpenTabs()])
  }
}

/** Squash a branch into the current one, staged, with the suggested message in the commit box. */
export async function mergeBranch(branch: string, into: string) {
  if (dirtyTabs().length) return toast('Save or close the files with unsaved edits before merging.', 'error')
  const ok = await confirmBox({
    title: `Merge ${branch} into ${into}?`,
    message: `Everything ${branch} changes is staged on ${into} as one change. Nothing is committed: review it in Changes, run the checks, then commit.`,
    ok: 'Stage the merge',
  })
  if (ok) await stageMerge(branch, into)
}

/** Promote a trial: back on the main branch, the testing branch is staged as one change (the merge), to check, commit and push. */
export async function promoteTrial(m: MachineDeployment) {
  if (!m.testing) return
  if (dirtyTabs().length) return toast('Save or close the files with unsaved edits before promoting.', 'error')
  const ok = await confirmBox({
    title: `Promote the trial on ${m.host} to ${m.main}?`,
    message: `This switches to ${m.main} and stages everything ${m.testing} changes as one change. Nothing is committed: run the checks, commit, then push, and every machine the change touches gets it.`,
    ok: 'Switch and stage',
  })
  if (ok) await stageMerge(m.testing, m.main, () => api.switchBranch(m.main))
}

async function stageMerge(branch: string, into: string, first?: () => Promise<unknown>) {
  repo.busy = 'merge'
  try {
    if (first) await first()
    draft.message = await api.merge(branch)
    toast(`${branch} is staged on ${into}. Check and commit it here.`, 'ok', 'Merge staged')
    go('#/changes')
  } catch (e) {
    if (e instanceof ApiError && e.status === 409) await alertBox({ title: 'Cannot merge', ...conflictText(e.message, 'merge') })
    else if (e instanceof ApiError && e.status === 400 && /nothing/.test(e.message)) toast(`${into} already has everything from ${branch}.`, 'info', 'Nothing to merge')
    else if (e instanceof ApiError && e.status === 404) toast(`There is no branch named ${branch} any more.`, 'error', 'Merge failed')
    else report(e, 'Merge failed')
  } finally {
    repo.busy = ''
    await Promise.all([refresh(), syncOpenTabs()])
    app.logVersion++
  }
}

const sentence = (s: string) => s.charAt(0).toUpperCase() + s.slice(1) + (/[.?!]$/.test(s) ? '' : '.')

/**
 * Try what is staged (or the unpushed commits) on one machine: the server commits it on the
 * machine's testing branch, on top of the current branch, and pushes it; the console moves there.
 */
export async function tryOnMachine(m: MachineDeployment) {
  if (repo.busy || !m.testing) return
  if (dirtyTabs().length) return toast('Save or close the files with unsaved edits before trying them.', 'error')
  const what = app.status?.files.some((f) => f.staged) ? 'the staged changes' : 'the commits not pushed yet'
  const ok = await confirmBox({
    title: `Try on ${m.host}?`,
    message: `This commits ${what} on ${m.testing}, on top of ${m.main}, and pushes it: comin deploys it on ${m.host} only, with switch-to-configuration test, so a reboot brings back what ${m.main} gave it. This console then works on ${m.testing}, where you try more, promote it to ${m.main} or end the trial.`,
    ok: `Try on ${m.host}`,
  })
  if (!ok || repo.busy) return
  repo.busy = 'try'
  const msg = draft.message.trim()
  try {
    let t
    try {
      t = await api.tryOn(m.host, msg)
    } catch (e) {
      if (!(e instanceof ApiError && e.status === 409 && /replace it\?$/.test(e.message))) throw e
      const again = await confirmBox({
        title: `Replace the trial on ${m.host}?`,
        message: `${sentence(e.message.replace(/: replace it\?$/, ''))} Replacing it drops those changes from ${m.testing}.`,
        ok: 'Replace it',
        danger: true,
      })
      if (!again) return
      t = await api.tryOn(m.host, msg, true)
    }
    draft.message = ''
    toast(`${t.commit.slice(0, 7)} is on ${t.branch}: comin deploys it on ${m.host} within a few minutes.`, 'ok', `Trying on ${m.host}`)
    go('#/changes')
  } catch (e) {
    if (!(e instanceof ApiError && e.status === 401)) await alertBox({ title: `Cannot try on ${m.host}`, message: sentence(message(e)) })
  } finally {
    repo.busy = ''
    await Promise.all([refresh(), syncOpenTabs(), loadDeployments()])
    app.logVersion++
  }
}

/** End a trial: the machine's testing branch goes back to its main branch, which comin deploys; this console too, when on it. */
export async function endTrial(m: MachineDeployment) {
  if (repo.busy || !m.testing) return
  const here = app.status?.branch === m.testing
  const ok = await confirmBox({
    title: `End the trial on ${m.host}?`,
    message: `${m.testing} is pointed back at the last commit of ${m.main} on the remote, so comin switches ${m.host} back to what ${m.main} gives it.${here ? ` This console goes back to ${m.main}; the trial's commits stay on its ${m.testing} branch.` : ''}`,
    ok: 'End the trial',
    danger: true,
  })
  if (!ok || repo.busy) return
  repo.busy = 'end'
  try {
    toast(await api.endTrial(m.host), 'ok', 'Trial ended')
    if (here) {
      try {
        await api.switchBranch(m.main)
      } catch (e) {
        report(e, `Still on ${m.testing}: commit or discard its changes, then switch to ${m.main}`)
      }
    }
  } catch (e) {
    report(e, `Cannot end the trial on ${m.host}`)
  } finally {
    repo.busy = ''
    await Promise.all([refresh(), syncOpenTabs(), loadDeployments()])
    app.logVersion++
  }
}

/** Roll a machine whose deployment failed back: revert the commits since the one it runs, start the checks, then commit and push. */
export async function rollBack(m: MachineDeployment) {
  const r = rollbackOf(m)
  if (!r || repo.busy) return
  if (app.status?.branch !== m.main)
    return alertBox({ title: 'Cannot revert here', message: `Switch to ${m.main} first: the revert goes on the branch comin deploys to ${m.host}.` })
  const ok = await confirmBox({ title: `${r.label}?`, message: r.question, pre: r.list || undefined, ok: 'Revert and check', wide: !!r.list })
  if (!ok || repo.busy) return
  repo.busy = 'revert'
  try {
    await api.revert(r.rev, r.since)
    draft.message = r.message
    await refresh()
    draft.pushTree = app.status?.staged_tree ?? ''
    go('#/changes')
    void checkStaged()
  } catch (e) {
    if (e instanceof ApiError && e.status === 409 && !/pull first/.test(e.message)) await alertBox({ title: 'Cannot revert', ...conflictText(e.message, 'revert') })
    else if (e instanceof ApiError && e.status === 409) await alertBox({ title: 'Cannot revert', message: sentence(e.message) })
    else report(e, 'Revert failed')
  } finally {
    repo.busy = ''
    await Promise.all([refresh(), syncOpenTabs()])
  }
}

/** The server's 409 reason, with what to do about it. */
function conflictText(reason: string, what: 'revert' | 'merge'): { message: string; pre?: string } {
  const files = app.status?.files.length ?? 0
  if (files) return { message: `The working copy has ${files} changed file${files === 1 ? '' : 's'}. Commit or discard ${files === 1 ? 'it' : 'them'} in Changes first, then ${what} again.` }
  const [first, ...rest] = reason.split('\n')
  return { message: `${first} Make the change by hand in Code instead.`, pre: rest.join('\n').trim() || undefined }
}
