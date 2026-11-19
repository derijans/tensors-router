//go:build linux

package hardware

import "os"

func platformDeviceProbes(detector Detector) []deviceProbe {
	return []deviceProbe{
		nvidiaSMIProbe{detector: detector},
		amdgpuSysfsProbe{sysfs: os.DirFS("/sys")},
		rocmSMIProbe{detector: detector},
	}
}
