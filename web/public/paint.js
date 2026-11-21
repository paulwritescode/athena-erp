// First paint: theme and cached Workspace appearance before Vue boots.
// Mirrors useTheme init and appearance.ts applyAppearance; the app
// re-applies authoritatively on load. A separate file (not inline) because
// the Content-Security-Policy allows scripts from 'self' only, and it runs
// before any paint because it is synchronous in <head>.
try {
  var theme = localStorage.getItem('athena-theme')
  var dark = theme === 'dark' || (theme !== 'light' && window.matchMedia('(prefers-color-scheme: dark)').matches)
  document.documentElement.classList.toggle('dark', dark)
  var cached = JSON.parse(localStorage.getItem('athena-appearance') || 'null')
  if (cached && cached.preset) {
    if (cached.preset === 'custom' && (cached.light || cached.accent)) {
      var light = cached.light || cached.accent
      var darkAccent = cached.dark || cached.accent
      var charts = (cached.chartColors || []).slice(0, 5).map(function (v, i) {
        return v ? ' --chart-' + (i + 1) + ': ' + v + ';' : ''
      }).join('')
      var chart1 = (cached.chartColors || [])[0] ? '' : ' --chart-1: var(--primary);'
      var css =
        ':root{--primary:' + light + ';--primary-foreground:oklch(0.985 0 0);' +
        '--sidebar-primary:' + light + ';--sidebar-primary-foreground:oklch(0.985 0 0);' + chart1 + charts + '}' +
        '.dark{--primary:' + darkAccent + ';--primary-foreground:oklch(0.21 0.006 285.885);' +
        '--sidebar-primary:' + darkAccent + ';--sidebar-primary-foreground:oklch(0.21 0.006 285.885);' + chart1 + charts + '}'
      var el = document.createElement('style')
      el.id = 'athena-appearance'
      el.textContent = css
      document.head.appendChild(el)
    } else if (cached.preset !== 'custom') {
      document.documentElement.dataset.accent = cached.preset
    }
  }
} catch (e) { /* A corrupt cache keeps the default look. */ }
