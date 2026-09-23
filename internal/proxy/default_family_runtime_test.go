package proxy

import "testing"

func defaultFamilyRuntime(t *testing.T, service *Service, readiness backendReadiness) *backendRuntime {
	t.Helper()
	runtime, err := service.runtimeForBackendMode(service.backendMode, readiness)
	if err != nil {
		t.Fatal(err)
	}
	if runtime == nil {
		t.Fatalf("backend mode %q has no runtime for readiness %v", service.backendMode, readiness)
	}
	return runtime
}
