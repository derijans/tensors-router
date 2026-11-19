//go:build !linux && !windows && !darwin

package hardware

func platformDeviceProbes(detector Detector) []deviceProbe {
	return vendorToolProbes(detector)
}
