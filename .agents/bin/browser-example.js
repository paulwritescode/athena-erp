// Example scenario for .agents/bin/browser. The driver already signed in
// through the invite challenge and set the requested theme before this runs.
//
//   suite -- /path/to/.agents/bin/browser /path/to/browser-example.js --shots /tmp/shots
//
// A scenario exports one async function over the helpers (see
// browser-README.md). Throw nothing on success; helpers throw AppError (exit
// 1) for app failures and HarnessError (exit 3) for driver problems.
module.exports = async ({ goto, shot, waitFor, setTheme, expectNoErrors }) => {
  await goto('/build/settings');
  await waitFor(`document.body.textContent.includes('Workspace preferences.')`, 'settings page');
  await shot('settings');
  await setTheme('dark');
  await goto('/build/settings');
  await waitFor(`document.body.textContent.includes('Workspace preferences.')`, 'settings page in dark');
  await shot('settings-dark');
  await expectNoErrors();
};
