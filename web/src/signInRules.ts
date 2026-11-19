// The sign-in checks the server also makes (internal/auth MinPasswordLength),
// repeated so an empty or too short entry shows before a request is sent.
export const minPasswordLength = 12

export type SignInProblems = { username?: string; password?: string }

export function signInProblems(username: string, password: string): SignInProblems {
  const problems: SignInProblems = {}
  if (!username.trim()) problems.username = 'Enter your username.'
  if (!password) problems.password = 'Enter your password.'
  else if ([...password].length < minPasswordLength) problems.password = `Passwords have at least ${minPasswordLength} characters.`
  return problems
}
