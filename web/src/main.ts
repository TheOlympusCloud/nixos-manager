import { mount } from 'svelte'
import './app.css'
import App from './App.svelte'
import { applyTheme } from './lib/theme.svelte'

applyTheme()

export default mount(App, { target: document.getElementById('app')! })
