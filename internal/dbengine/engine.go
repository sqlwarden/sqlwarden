package dbengine

import "context"

type facadeEngine struct {
	reg Registration
}

func (e *facadeEngine) ID() EngineID        { return e.reg.ID }
func (e *facadeEngine) DisplayName() string { return e.reg.DisplayName }
func (e *facadeEngine) Dialect() Dialect    { return e.reg.Dialect }

func (e *facadeEngine) Capabilities() CapabilitySet {
	caps, spec := capabilitiesOf(e.reg)
	return CapabilitySet{
		Engine:       EngineDescriptor{ID: e.reg.ID, DisplayName: e.reg.DisplayName, Dialect: e.reg.Dialect},
		Capabilities: caps,
		Schema:       spec,
	}
}

func (e *facadeEngine) Open(ctx context.Context, cfg ConnectionConfig) (Connection, error) {
	d := e.reg.NewDriver()
	if err := d.Connect(ctx, cfg); err != nil {
		return nil, err
	}
	// driver.Driver's method set is a superset of Connection's, so the connected
	// driver is returned directly; optional capabilities resolve via assertion.
	return d, nil
}
