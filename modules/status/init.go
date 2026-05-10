package status

import "fyshos.com/tyde"

func init() {
	// system area (bottom of widget panel) - order is top to bottom
	tyde.RegisterModule(networkMeta)
	tyde.RegisterModule(batteryMeta)
	tyde.RegisterModule(soundMeta)
	tyde.RegisterModule(brightnessMeta)
}
