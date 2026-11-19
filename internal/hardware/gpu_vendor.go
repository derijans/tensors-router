package hardware

import (
	"strconv"
	"strings"
)

const (
	pciVendorNVIDIA    = 0x10de
	pciVendorAMD       = 0x1002
	pciVendorIntel     = 0x8086
	pciVendorRedHat    = 0x1af4
	pciVendorMicrosoft = 0x1414
	pciVendorVMware    = 0x15ad
)

func vendorFromPCIID(vendorID uint32) string {
	switch vendorID {
	case pciVendorNVIDIA:
		return GPUVendorNVIDIA
	case pciVendorAMD:
		return GPUVendorAMD
	case pciVendorIntel:
		return GPUVendorIntel
	case pciVendorRedHat, pciVendorMicrosoft, pciVendorVMware:
		return GPUVendorVirtual
	default:
		return GPUVendorOther
	}
}

func parsePCIVendorID(text string) (uint32, bool) {
	text = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(text)), "0x")
	value, err := strconv.ParseUint(text, 16, 32)
	return uint32(value), err == nil
}
