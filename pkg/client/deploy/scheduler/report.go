package scheduler

import (
	"fmt"
	"strings"
)

// SchedulingReport provides detailed information about scheduling decisions.
type SchedulingReport struct {
	// Eligible machines that passed all constraints.
	Eligible []*Machine
	// Ineligible machines with their failure reasons.
	Ineligible []MachineEvaluation
}

// MachineEvaluation holds constraint results for a single machine.
type MachineEvaluation struct {
	Machine *Machine
	Results []ConstraintResult
}

// Failed returns constraint results that did not pass.
func (e MachineEvaluation) Failed() []ConstraintResult {
	var failed []ConstraintResult
	for _, r := range e.Results {
		if !r.Satisfied {
			failed = append(failed, r)
		}
	}
	return failed
}

// Passed returns true if all constraints were satisfied.
func (e MachineEvaluation) Passed() bool {
	for _, r := range e.Results {
		if !r.Satisfied {
			return false
		}
	}
	return true
}

// Error formats the report as a user-friendly error message showing why
// each ineligible machine failed scheduling constraints.
func (r *SchedulingReport) Error() string {
	if len(r.Ineligible) == 0 {
		return "no machines in cluster"
	}

	var sb strings.Builder
	for i, eval := range r.Ineligible {
		if i > 0 {
			sb.WriteString("\n")
		}

		machineName := eval.Machine.Info.Name
		if machineName == "" {
			machineName = eval.Machine.Info.Id
		}

		failed := eval.Failed()
		if len(failed) == 0 {
			sb.WriteString(fmt.Sprintf("  %s: passed all constraints", machineName))
			continue
		}

		// Format each failure reason.
		var reasons []string
		for _, f := range failed {
			reasons = append(reasons, f.Reason)
		}

		sb.WriteString(fmt.Sprintf("  %s: %s", machineName, strings.Join(reasons, "; ")))
	}

	return sb.String()
}

// HasEligible returns true if at least one machine is eligible.
func (r *SchedulingReport) HasEligible() bool {
	return len(r.Eligible) > 0
}

// Summary returns a brief summary of the scheduling result.
func (r *SchedulingReport) Summary() string {
	return fmt.Sprintf("%d eligible, %d ineligible", len(r.Eligible), len(r.Ineligible))
}
