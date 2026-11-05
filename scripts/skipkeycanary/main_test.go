//go:build skipkeycanary

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"bytes"
	"context"
	"os"
	"slices"
	"strings"
	"syscall"
	"testing"
)

type cancelOnUpload struct {
	*fakeAPI
	deliver func()
}

func (c *cancelOnUpload) UploadToken(ctx context.Context, name string, token []byte) (string, error) {
	id, err := c.fakeAPI.UploadToken(ctx, name, token)
	c.deliver()
	return id, err
}

func (c *cancelOnUpload) WaitInstanceSynced(ctx context.Context, _ string) error {
	return ctx.Err()
}

func TestRunCleansUpWhenASignalCancelsTheRun(t *testing.T) {
	f := newFake()
	f.uploadID = "50"

	var asked []os.Signal
	var signalled context.CancelFunc
	originalNotify, originalConnect := notifyContext, connectAPI
	t.Cleanup(func() {
		notifyContext = originalNotify
		connectAPI = originalConnect
	})
	notifyContext = func(parent context.Context, sigs ...os.Signal) (context.Context, context.CancelFunc) {
		asked = sigs
		ctx, cancel := context.WithCancel(parent)
		signalled = cancel
		return ctx, cancel
	}
	connectAPI = func(func(string) string) (API, []byte, error) {
		return &cancelOnUpload{fakeAPI: f, deliver: func() { signalled() }}, []byte("t"), nil
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"-no-issues"}, func(string) string { return "" }, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("exit code = %d, want 1; stderr:\n%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "context canceled") {
		t.Fatalf("stderr does not report the cancellation:\n%s", stderr.String())
	}
	if !slices.Contains(asked, os.Interrupt) || !slices.Contains(asked, os.Signal(syscall.SIGTERM)) {
		t.Fatalf("run asked for signals %v, want SIGINT and SIGTERM", asked)
	}
	if len(f.instances) != 0 || len(f.prestages[Computer])+len(f.prestages[Mobile]) != 0 {
		t.Fatalf("a signalled run left objects behind: %+v %+v", f.instances, f.prestages)
	}
	if len(f.deletedInst) != 1 || f.deletedInst[0] != "50" {
		t.Fatalf("deleted instances = %v", f.deletedInst)
	}
}
