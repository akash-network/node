package types

// func TestResourceUnitsSubIsIdempotent(t *testing.T) {
// 	ru := ResourceUnits{
// 		CPU:    &CPU{Units: NewResourceValue(1000)},
// 		Memory: &Memory{Quantity: NewResourceValue(10 * unit.Gi)},
// 		Storage: Volumes{
// 			Storage{Quantity: NewResourceValue(10 * unit.Gi)},
// 		},
// 	}
// 	cpuAsString := ru.CPU.String()
// 	newRu, err := ru.Sub(
// 		ResourceUnits{
// 			CPU:    &CPU{Units: NewResourceValue(1)},
// 			Memory: &Memory{Quantity: NewResourceValue(0 * unit.Gi)},
// 			Storage: Volumes{
// 				Storage{Quantity: NewResourceValue(0 * unit.Gi)},
// 			},
// 		},
// 	)
// 	require.NoError(t, err)
// 	require.NotNil(t, newRu)
//
// 	cpuAsStringAfter := ru.CPU.String()
// 	require.Equal(t, cpuAsString, cpuAsStringAfter)
//
// 	require.Equal(t, newRu.CPU.GetUnits().Value(), uint64(999))
// }
