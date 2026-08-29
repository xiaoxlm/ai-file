package agent

// Goal identifies the single file an agent run must analyze.
type Goal struct {
	path string
}

// NewGoal returns an immutable goal for path.
func NewGoal(path string) Goal {
	return Goal{path: path}
}

// Path returns the target file path.
func (g Goal) Path() string {
	return g.path
}
