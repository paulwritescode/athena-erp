// The Athena ASCII wordmark: five lines, 41 columns, plain ASCII only. It is
// the sign-in footer mark and the CLI banner; cmd/athena/main.go keeps a copy
// of the same drawing, so change both together. Section 9.6 allows at most
// eight lines.
export const ATHENA_MARK = [
  '   _    ___  _   _  ___  _   _    _',
  '  / \\    |  | | | | |    | \\ | |  / \\',
  ' / _ \\   |  | |_| | |__  |  \\| | / _ \\',
  '/ /_\\ \\  |  |  _  | |    | |\\  | / /_\\ \\',
  '|_| |_| _|_ |_| |_| |___| |_| \\_| |_| |_|',
].join('\n')
