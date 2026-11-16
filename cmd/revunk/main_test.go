package main

import "testing"

func TestRunHelpSucceeds(t *testing.T) {
	for _, arg := range []string{"help", "-h", "--help"} {
		if err := run([]string{arg}); err != nil {
			t.Errorf("run(%q) = %v, want nil", arg, err)
		}
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	if err := run([]string{"serve"}); err == nil {
		t.Fatal("expected an error for an unknown command")
	}
}
