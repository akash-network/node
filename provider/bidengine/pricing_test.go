package bidengine

// func TestCalculatePriceRange(t *testing.T) {

// 	tests := []struct {
// 		name      string
// 		resources []types.ResourceGroup
// 		min       uint64
// 		max       uint64
// 	}{
// 		{
// 			name: "max-unbound",
// 			min:  25600,
// 			max:  76800,
// 			resources: []types.ResourceGroup{
// 				{
// 					Unit: types.ResourceUnit{
// 						CPU:    100,
// 						Memory: 512 * unit.Gi,
// 						Disk:   512 * unit.Mi,
// 					},
// 					Count: 1,
// 					Price: 1000000,
// 				},
// 			},
// 		},
// 		{
// 			name: "max-truncated",
// 			min:  25600,
// 			max:  25601,
// 			resources: []types.ResourceGroup{
// 				{
// 					Unit: types.ResourceUnit{
// 						CPU:    100,
// 						Memory: 512 * unit.Gi,
// 						Disk:   512 * unit.Mi,
// 					},
// 					Count: 1,
// 					Price: 25601,
// 				},
// 			},
// 		},
// 		{
// 			name: "min-max-same",
// 			min:  25,
// 			max:  25,
// 			resources: []types.ResourceGroup{
// 				{
// 					Unit: types.ResourceUnit{
// 						CPU:    100,
// 						Memory: 512 * unit.Mi,
// 						Disk:   512 * unit.Mi,
// 					},
// 					Count: 1,
// 					Price: 25,
// 				},
// 			},
// 		},
// 		{
// 			name: "pass-by-one",
// 			min:  25,
// 			max:  26,
// 			resources: []types.ResourceGroup{
// 				{
// 					Unit: types.ResourceUnit{
// 						CPU:    100,
// 						Memory: 512 * unit.Mi,
// 						Disk:   512 * unit.Mi,
// 					},
// 					Count: 1,
// 					Price: 26,
// 				},
// 			},
// 		},
// 	}

// 	for _, test := range tests {
// 		rlist := &types.DeploymentGroup{Resources: test.resources}

// 		min, max := calculatePriceRange(rlist)
// 		assert.Equal(t, test.min, min, "%v:min=%v", test.name, min)
// 		assert.Equal(t, test.max, max, "%v:max=%v", test.name, max)
// 	}

// }
