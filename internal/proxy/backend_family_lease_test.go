package proxy

import (
	"context"
	"testing"
	"time"

	"tensors-router/internal/backendmode"
)

const familySwitchBlockedProbe = 150 * time.Millisecond

func TestFamilySwitchWaitsForLeaseHolderOfCurrentFamily(t *testing.T) {
	service := newTwoFamilyTestService(t)
	releaseKobold, err := service.leaseBackendFamily(context.Background(), backendmode.Kobold)
	if err != nil {
		t.Fatal(err)
	}

	switched := make(chan error, 1)
	go func() {
		release, err := service.leaseBackendFamily(context.Background(), backendmode.LlamaSDCPP)
		if err == nil {
			release()
		}
		switched <- err
	}()

	select {
	case err := <-switched:
		t.Fatalf("family switched while the current family was still leased: err=%v", err)
	case <-time.After(familySwitchBlockedProbe):
	}

	releaseKobold()
	select {
	case err := <-switched:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("family switch did not proceed after the lease was released")
	}
	if got := service.currentBackendMode(); got != backendmode.LlamaSDCPP {
		t.Fatalf("current backend mode = %q, want %q", got, backendmode.LlamaSDCPP)
	}
}

func TestFamilySwitchGivesNewRequestsNoLeaseWhileSwitching(t *testing.T) {
	service := newTwoFamilyTestService(t)
	releaseKobold, err := service.leaseBackendFamily(context.Background(), backendmode.Kobold)
	if err != nil {
		t.Fatal(err)
	}
	switched := make(chan error, 1)
	go func() {
		release, err := service.leaseBackendFamily(context.Background(), backendmode.LlamaSDCPP)
		if err == nil {
			release()
		}
		switched <- err
	}()
	waitForFamilySwitching(t, service)

	lateContext, cancel := context.WithTimeout(context.Background(), familySwitchBlockedProbe)
	defer cancel()
	if _, err := service.leaseBackendFamily(lateContext, backendmode.Kobold); err == nil {
		t.Fatal("a request for the outgoing family took a lease while a switch was pending")
	}

	releaseKobold()
	if err := <-switched; err != nil {
		t.Fatal(err)
	}
}

func waitForFamilySwitching(t *testing.T, service *Service) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		state := service.backendSwitch
		state.mu.Lock()
		switching := state.switching
		state.mu.Unlock()
		if switching {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("family switch never started")
}
