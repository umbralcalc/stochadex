package simulator

// CopySettingsForPartitions copies the settings for only a specified
// subset of state partition indices.
func CopySettingsForPartitions(partitionIndices []int, settings *Settings) *Settings {
	settingsCopy := &Settings{}
	settingsCopy.InitTimeValue = settings.InitTimeValue
	settingsCopy.TimestepsHistoryDepth = settings.TimestepsHistoryDepth
	for _, index := range partitionIndices {
		paramsCopy := settings.Params[index]
		settingsCopy.Params = append(
			settingsCopy.Params,
			paramsCopy,
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
