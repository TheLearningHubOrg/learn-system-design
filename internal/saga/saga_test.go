package saga

import (
	"context"
	"errors"
	"slices"
	"testing"
)

func TestFailedStepCompensatesInReverse(t *testing.T) {
	var log []string
	step := func(name string, fail bool) Step {
		return Step{
			Name: name,
			Execute: func(context.Context) error {
				log = append(log, "do "+name)
				if fail {
					return errors.New("card declined")
				}
				return nil
			},
			Compensate: func(context.Context) error {
				log = append(log, "undo "+name)
				return nil
			},
		}
	}
	err := Run(context.Background(), []Step{
		step("create-order", false),
		step("reserve-stock", false),
		step("charge-card", true),
		step("ship", false),
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	want := []string{"do create-order", "do reserve-stock", "do charge-card", "undo reserve-stock", "undo create-order"}
	if !slices.Equal(log, want) {
		t.Fatalf("got %v, want %v", log, want)
	}
}

func TestCompensationFailureIsReported(t *testing.T) {
	refundFailed := errors.New("refund API down")
	err := Run(context.Background(), []Step{
		{Name: "charge", Execute: func(context.Context) error { return nil }, Compensate: func(context.Context) error { return refundFailed }},
		{Name: "ship", Execute: func(context.Context) error { return errors.New("no courier") }},
	})
	if !errors.Is(err, refundFailed) {
		t.Fatalf("compensation failure hidden: %v", err)
	}
}

func TestAllStepsSucceed(t *testing.T) {
	ok := func(context.Context) error { return nil }
	if err := Run(context.Background(), []Step{{Name: "a", Execute: ok}, {Name: "b", Execute: ok}}); err != nil {
		t.Fatal(err)
	}
}
