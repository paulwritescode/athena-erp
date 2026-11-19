package files

// SetFault makes the store call f at its write stages; a non-nil error from f
// fails the stage. Tests use it to interrupt a write.
func (s *Store) SetFault(f func(stage string) error) { s.fault = f }
