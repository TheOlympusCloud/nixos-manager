// Night (the default) or Print (greenbar paper, for daylight); the choice is kept in this browser.
export type Theme = 'night' | 'print'
const KEY = 'nixmgr.theme'

export const theme = $state({ name: (typeof localStorage !== 'undefined' && localStorage.getItem(KEY) === 'print' ? 'print' : 'night') as Theme })

export function applyTheme(name: Theme = theme.name) {
  theme.name = name
  if (name === 'print') document.documentElement.dataset.theme = 'print'
  else delete document.documentElement.dataset.theme
  localStorage.setItem(KEY, name)
}
