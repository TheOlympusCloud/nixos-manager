// Repository actions shared by the top bar, History, the command palette and Changes.
import { ApiError, api } from './api'
import { go } from './router.svelte'
import { app, fileTabs, refresh, syncOpenTabs } from './state.svelte'
import { alertBox, confirmBox, report, toast } from './ui.svelte'

/** The commit message being written in Changes; survives leaving the view, and merge or revert can prefill it. */
export const draft = $state({ message: '' })

export const repo = $state({ busy: '' as '' | 'pull' | 'push' | 'revert' | 'merge' | 'switch' | 'branch' })

/** Why Pull and Push cannot work, or '' when they can. */
export const noRemote = () => (app.info && !app.info.remote ? 'No remote is configured: set NIXMGR_REMOTE and restart the server (see Settings).' : '')

const message = (e: unknown) => (e instanceof Error ? e.message : String(e))

export async function pullOrPush(kind: 'pull' | 'push') {
  if (repo.busy) return
  const why = noRemote()
  if (why) return toast(why, 'info', kind === 'pull' ? 'Cannot pull' : 'Cannot push', { label: 'Open Settings', run: () => go('#/settings') })
  repo.busy = kind
  try {
    let out: string
    try {
      out = await api[kind]()
    } catch (e) {
      // 412: the commit to push was never checked as a whole.
      if (!(kind === 'push' && e instanceof ApiError && e.status === 412)) throw e
      const force = await confirmBox({
        title: 'Push an unchecked commit?',
        message:
          'The commit at the tip of this branch has not passed a check as a whole. Run the checks with nothing staged or changed to check it, or push it anyway: the machines will deploy it unchecked.',
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
  if (!ok) return
  repo.busy = 'merge'
  try {
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

/** The server's 409 reason, with what to do about it. */
function conflictText(reason: string, what: 'revert' | 'merge'): { message: string; pre?: string } {
  const files = app.status?.files.length ?? 0
  if (files) return { message: `The working copy has ${files} changed file${files === 1 ? '' : 's'}. Commit or discard ${files === 1 ? 'it' : 'them'} in Changes first, then ${what} again.` }
  const [first, ...rest] = reason.split('\n')
  return { message: `${first} Make the change by hand in Code instead.`, pre: rest.join('\n').trim() || undefined }
}
