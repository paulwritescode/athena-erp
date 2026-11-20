// The sign-in checks the server also makes (internal/auth MinPasswordLength),
// repeated so an empty or too short entry shows before a request is sent.
export const minPasswordLength = 12
export const maxPasswordLength = 256

export type SignInProblems = { username?: string; password?: string }

export function signInProblems(username: string, password: string): SignInProblems {
  const problems: SignInProblems = {}
  if (!username.trim()) problems.username = 'Enter your username.'
  if (!password) problems.password = 'Enter your password.'
  else if ([...password].length < minPasswordLength) problems.password = `Passwords have at least ${minPasswordLength} characters.`
  return problems
}

export type ChallengeProblems = { username?: string; challenge?: string; password?: string; confirm?: string }

// The completion checks the server also makes (internal/auth CheckPassword
// plus a matching confirmation), repeated before a request is sent.
export function challengeProblems(username: string, challenge: string, password: string, confirm: string): ChallengeProblems {
  const problems: ChallengeProblems = {}
  if (!username.trim()) problems.username = 'Enter your username.'
  if (!challenge.trim()) problems.challenge = 'Enter the challenge.'
  if (!password) problems.password = 'Choose a password.'
  else if ([...password].length < minPasswordLength || [...password].length > maxPasswordLength) {
    problems.password = `Passwords need ${minPasswordLength} to ${maxPasswordLength} characters.`
  } else if (password.toLowerCase() === username.trim().toLowerCase()) {
    problems.password = 'The password may not be the username.'
  } else if (password !== confirm) {
    problems.confirm = 'The passwords do not match.'
  }
  return problems
}
