// Package saga runs an orchestrated saga (Stage 4): a sequence of local
// transactions where each step has a compensating action that undoes it.
package saga

import (
	"context"
	"errors"
	"fmt"
)

// Step is one local transaction plus the action that semantically undoes it.
type Step struct {
	Name       string
	Execute    func(ctx context.Context) error
	Compensate func(ctx context.Context) error
}

// Run executes steps in order. If a step fails, the steps that already
// succeeded are compensated in reverse order. The returned error joins the
// original failure with any compensation failures; those need repair,
// because the system is now neither fully done nor fully undone.
func Run(ctx context.Context, steps []Step) error {
	done := make([]Step, 0, len(steps))
	for _, s := range steps {
		if err := s.Execute(ctx); err != nil {
			errs := []error{fmt.Errorf("step %s: %w", s.Name, err)}
			for i := len(done) - 1; i >= 0; i-- {
				if done[i].Compensate == nil {
					continue
				}
				if cerr := done[i].Compensate(ctx); cerr != nil {
					errs = append(errs, fmt.Errorf("compensate %s: %w", done[i].Name, cerr))
				}
			}
			return errors.Join(errs...)
		}
		done = append(done, s)
	}
	return nil
}
