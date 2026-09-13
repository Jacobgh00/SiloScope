package app

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunRejectsUnknownFlag(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run(context.Background(), []string{"--wat"}, &stdout, &stderr)

	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2", exitCode)
	}

	if stderr.String() == "" {
		t.Fatal("stderr is empty, want usage error")
	}
}

func TestRunPrintsHelpWithSuccess(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run(context.Background(), []string{"--help"}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}

	if !strings.Contains(stderr.String(), "-repo") || !strings.Contains(stderr.String(), "-since") {
		t.Fatalf("help output = %q, want -repo and -since flags", stderr.String())
	}
}
