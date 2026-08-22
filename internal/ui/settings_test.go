package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"fyshos.com/tyde"
	"fyshos.com/tyde/modules/keyboard"
	"fyshos.com/tyde/modules/status"
	"fyshos.com/tyde/test"
)

func TestDeskSettings_IsModuleEnabled(t *testing.T) {
	s := test.NewSettings()
	s.SetModuleNames([]string{"Yes", "maybe"})

	assert.True(t, isModuleEnabled("Yes", s))
	assert.True(t, isModuleEnabled("maybe", s))
	assert.False(t, isModuleEnabled("Maybe", s))
	assert.False(t, isModuleEnabled("No", s))
}

func TestModulesForComputer(t *testing.T) {
	mods := []string{"Sound", status.BatteryModuleName, status.BrightnessModuleName, keyboard.ModuleName}

	assert.Equal(t, []string{"Sound"},
		modulesForComputer(tyde.ComputerDesktop, mods))
	assert.Equal(t, []string{"Sound", status.BatteryModuleName, status.BrightnessModuleName},
		modulesForComputer(tyde.ComputerLaptop, mods))
	assert.Equal(t, []string{"Sound", status.BatteryModuleName, status.BrightnessModuleName, keyboard.ModuleName},
		modulesForComputer(tyde.ComputerTablet, mods))

	// a machine gaining the hardware turns the modules back on
	assert.Equal(t, []string{"Sound", status.BatteryModuleName, status.BrightnessModuleName, keyboard.ModuleName},
		modulesForComputer(tyde.ComputerTablet, []string{"Sound"}))
}
