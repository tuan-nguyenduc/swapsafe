package cli

// Exit codes, as promised in PLAN section 5.6. They are part of the CLI
// contract: CI pipelines branch on them.
const (
	// ExitOK: no finding reached the --fail-on threshold.
	ExitOK = 0
	// ExitError: swapsafe could not finish the run.
	ExitError = 1
	// ExitFindings: the run finished and found something at or above --fail-on.
	ExitFindings = 2
)

// ExitError carries a specific process exit code out of a command.
type exitError struct {
	code int
	msg  string
}

func (e exitError) Error() string { return e.msg }

// Code reports the process exit status this error asks for.
func (e exitError) Code() int { return e.code }

// Coder is implemented by errors that name their own exit code.
type Coder interface {
	error
	Code() int
}
