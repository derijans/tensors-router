package downloader

import (
	"strconv"

	"tensors-router/internal/flatyaml"
)

var downloaderConfigDialect = flatyaml.Dialect{
	Scalar:               downloaderConfigString,
	NamesUnknownSections: true,
}

func (cfg *Config) schema() flatyaml.Schema {
	return flatyaml.Schema{
		"storage": {Fields: flatyaml.Fields{
			"root":                  flatyaml.String(&cfg.Storage.Root),
			"state_dir":             flatyaml.String(&cfg.Storage.StateDir),
			"database_path":         flatyaml.String(&cfg.Storage.DatabasePath),
			"free_space_reserve_gb": flatyaml.Int64(&cfg.Storage.FreeSpaceReserveGB),
		}},
		"huggingface": {Fields: flatyaml.Fields{
			"token":    flatyaml.String(&cfg.HuggingFace.Token),
			"endpoint": flatyaml.String(&cfg.HuggingFace.Endpoint),
		}},
		"downloads": {Fields: flatyaml.Fields{
			"concurrent_jobs":  nativeInt(&cfg.Downloads.ConcurrentJobs),
			"concurrent_files": nativeInt(&cfg.Downloads.ConcurrentFiles),
			"retry_limit":      nativeInt(&cfg.Downloads.RetryLimit),
			"timeout":          flatyaml.Duration(&cfg.Downloads.Timeout),
			"stall_timeout":    flatyaml.Duration(&cfg.Downloads.StallTimeout),
		}},
		"scanning": {Fields: flatyaml.Fields{
			"hash_workers":        nativeInt(&cfg.Scanning.HashWorkers),
			"write_hash_sidecars": flatyaml.Bool(&cfg.Scanning.WriteHashSidecars),
		}},
		"hardware": {Fields: flatyaml.Fields{
			"default_context":       nativeInt(&cfg.Hardware.DefaultContext),
			"vram_reserve_mb":       flatyaml.Int64(&cfg.Hardware.VRAMReserveMB),
			"safety_margin_percent": nativeInt(&cfg.Hardware.SafetyMarginPercent),
		}},
		"logging": {Fields: flatyaml.Fields{
			"mode": flatyaml.String(&cfg.Logging.Mode),
		}},
	}
}

func nativeInt(target *int) flatyaml.Field {
	return flatyaml.Scalar(func(value string) error {
		parsed, err := strconv.ParseInt(value, 10, strconv.IntSize)
		if err != nil {
			return err
		}
		*target = int(parsed)
		return nil
	})
}
