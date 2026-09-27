package proxy

import (
	"context"
	"errors"
	"sync"

	"tensors-router/internal/offloadsettings"
)

var errLendingSettingsReadOnly = errors.New("lending settings cannot be edited: this router has no settings store")

type lendingOverrideStore interface {
	Overrides(ctx context.Context) (offloadsettings.Values, error)
	Set(ctx context.Context, values offloadsettings.Values) error
	Clear(ctx context.Context, key string) error
	ClearAll(ctx context.Context) error
}

type lendingSettings struct {
	fileValues offloadsettings.Values
	store      lendingOverrideStore
	apply      func(offloadsettings.Settings)

	mu         sync.Mutex
	resolution offloadsettings.Resolution

	deliveryFailures *deliveryFailures
}

func newLendingSettings(fileValues offloadsettings.Values, store lendingOverrideStore, apply func(offloadsettings.Settings)) *lendingSettings {
	return &lendingSettings{
		fileValues: fileValues,
		store:      store,
		apply:      apply,
		resolution: offloadsettings.Resolve(fileValues, nil),

		deliveryFailures: newDeliveryFailures(),
	}
}

func (settings *lendingSettings) snapshot() offloadsettings.Resolution {
	settings.mu.Lock()
	defer settings.mu.Unlock()
	return settings.resolution
}

func (settings *lendingSettings) applyDatabaseLayer(ctx context.Context) (offloadsettings.Resolution, error) {
	overrides := offloadsettings.Values{}
	if settings.store != nil {
		stored, err := settings.store.Overrides(ctx)
		if err != nil {
			return settings.snapshot(), err
		}
		overrides = stored
	}
	resolution := offloadsettings.Resolve(settings.fileValues, overrides)
	settings.install(resolution)
	return resolution, nil
}

func (settings *lendingSettings) set(ctx context.Context, values offloadsettings.Values) (offloadsettings.Resolution, error) {
	if settings.store == nil {
		return settings.snapshot(), errLendingSettingsReadOnly
	}
	if err := settings.store.Set(ctx, values); err != nil {
		return settings.snapshot(), err
	}
	return settings.applyDatabaseLayer(ctx)
}

func (settings *lendingSettings) clear(ctx context.Context, key string) (offloadsettings.Resolution, error) {
	if settings.store == nil {
		return settings.snapshot(), errLendingSettingsReadOnly
	}
	var err error
	if key == "" {
		err = settings.store.ClearAll(ctx)
	} else {
		err = settings.store.Clear(ctx, key)
	}
	if err != nil {
		return settings.snapshot(), err
	}
	return settings.applyDatabaseLayer(ctx)
}

func (settings *lendingSettings) adoptFromMaster(values offloadsettings.Values) error {
	adopted, err := offloadsettings.Parse(withoutKeysFromNewerBuilds(values))
	if err != nil {
		return err
	}
	settings.install(offloadsettings.Resolution{Settings: adopted})
	return nil
}

func (settings *lendingSettings) install(resolution offloadsettings.Resolution) {
	settings.mu.Lock()
	settings.resolution = resolution
	settings.mu.Unlock()
	if settings.apply != nil {
		settings.apply(resolution.Settings)
	}
}

func withoutKeysFromNewerBuilds(values offloadsettings.Values) offloadsettings.Values {
	known := offloadsettings.Values{}
	for key, value := range values {
		if offloadsettings.Known(key) {
			known[key] = value
		}
	}
	return known
}
