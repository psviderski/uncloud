package operation

import (
	"context"
	"fmt"
	"strings"
)

// SequenceOperation is a composite operation that executes a sequence of operations in order.
type SequenceOperation struct {
	Operations []Operation
}

func (o *SequenceOperation) Execute(ctx context.Context, cli Client) error {
	for _, op := range o.Operations {
		if err := op.Execute(ctx, cli); err != nil {
			return err
		}
	}
	return nil
}

func (o *SequenceOperation) Format(resolver NameResolver) string {
	ops := make([]string, len(o.Operations))
	for i, op := range o.Operations {
		ops[i] = "- " + op.Format(resolver)
	}

	return strings.Join(ops, "\n")
}

func (o *SequenceOperation) String() string {
	ops := make([]string, len(o.Operations))
	for i, op := range o.Operations {
		ops[i] = op.String()
	}

	return fmt.Sprintf("SequenceOperation[%s]", strings.Join(ops, ", "))
}
