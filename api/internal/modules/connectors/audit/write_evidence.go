package audit

import "errors"

// decisionNotWritten identifies a rejected insert, never an uncertain commit.
// It does not prove that an earlier attempt with the same identity did not run.
type decisionNotWritten struct{ error }

func (err decisionNotWritten) Unwrap() error { return err.error }

func DecisionDefinitelyNotWritten(err error) bool {
	var rejected decisionNotWritten
	return errors.As(err, &rejected)
}
