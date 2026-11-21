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
      var customCharts = cached.chartPalette === 'custom' || (!cached.chartPalette && (cached.chartColors || []).length > 0)
      var stored = customCharts ? (cached.chartColors || []) : []
      var charts = stored.slice(0, 5).map(function (v, i) {
        return v ? ' --chart-' + (i + 1) + ': ' + v + ';' : ''
      }).join('')
      var css =
        ':root{--primary:' + light + ';--primary-foreground:oklch(0.985 0 0);' +
        '--sidebar-primary:' + light + ';--sidebar-primary-foreground:oklch(0.985 0 0);' + charts + '}' +
        '.dark{--primary:' + darkAccent + ';--primary-foreground:oklch(0.21 0.006 285.885);' +
        '--sidebar-primary:' + darkAccent + ';--sidebar-primary-foreground:oklch(0.21 0.006 285.885);' + charts + '}'
      var el = document.createElement('style')
      el.id = 'athena-appearance'
      el.textContent = css
      document.head.appendChild(el)
    } else if (cached.preset !== 'custom') {
      document.documentElement.dataset.accent = cached.preset
      if (!cached.chartPalette && (cached.chartColors || []).length > 0) {
        var legacy = (cached.chartColors || []).slice(0, 5).map(function (v, i) {
          return v ? ' --chart-' + (i + 1) + ': ' + v + ';' : ''
        }).join('')
        var legacyEl = document.createElement('style')
        legacyEl.id = 'athena-appearance'
        legacyEl.textContent = ':root{' + legacy + '}' + '.dark{' + legacy + '}'
        document.head.appendChild(legacyEl)
      }
    }
    if (cached.chartPalette && cached.chartPalette !== 'match' && cached.chartPalette !== 'custom') {
      document.documentElement.dataset.charts = cached.chartPalette
    }
  }
} catch (e) { /* A corrupt cache keeps the default look. */ }
