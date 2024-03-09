package params

import "github.com/umbralcalc/stochadex/pkg/simulator"

// NewParamsCopy is a convenience function which copies the input
// []*simulator.OtherParams to help ensure thread safety.
func NewParamsCopy(params []*simulator.OtherParams) []*simulator.OtherParams {
	paramsCopy := make([]*simulator.OtherParams, 0)
	for i := range params {
		p := *params[i]
		paramsCopy = append(paramsCopy, &p)
	}
	return paramsCopy
}

// CopySettingsForPartitions copies the settings for only a specified
// subset of state partition indices.
func CopySettingsForPartitions(
	partitionIndices []int,
	settings *simulator.Settings,
) *simulator.Settings {
	settingsCopy := &simulator.Settings{}
	settingsCopy.InitTimeValue = settings.InitTimeValue
	settingsCopy.TimestepsHistoryDepth = settings.TimestepsHistoryDepth
	for _, index := range partitionIndices {
		paramsCopy := *settings.OtherParams[index]
		settingsCopy.OtherParams = append(
			settingsCopy.OtherParams,
			&paramsCopy,
		)
		settingsCopy.InitStateValues = append(
			settingsCopy.InitStateValues,
			settings.InitStateValues[index],
		)
		settingsCopy.Seeds = append(
			settingsCopy.Seeds,
			settings.Seeds[index],
		)
		settingsCopy.StateWidths = append(
			settingsCopy.StateWidths,
			settings.StateWidths[index],
		)
		settingsCopy.StateHistoryDepths = append(
			settingsCopy.StateHistoryDepths,
			settings.StateHistoryDepths[index],
		)
	}
	return settingsCopy
}
