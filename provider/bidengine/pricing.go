package bidengine

// TODO: price

// func calculatePrice(resources types.ResourceGroup) uint64 {
// 	min, max := calculatePriceRange(resources)

// 	if max == min {
// 		return max
// 	}

// 	return uint64(rand.Int63n(int64(max-min)) + int64(min))
// }

// func calculatePriceRange(resources types.ResourceGroup) (uint64, uint64) {
// 	// TODO: catch overflow
// 	var (
// 		mem  uint64
// 		rmax uint64
// 	)

// 	cfg := validation.Config()

// 	for _, group := range resources.GetResources() {
// 		rmax += group.Price * uint64(group.Count)
// 		mem += group.Unit.Memory * uint64(group.Count)
// 	}

// 	cmin := uint64(float64(mem) * float64(cfg.MinGroupMemPrice) / float64(unit.Gi))
// 	cmax := uint64(float64(mem) * float64(cfg.MaxGroupMemPrice) / float64(unit.Gi))

// 	if cmax > rmax {
// 		cmax = rmax
// 	}
// 	if cmax == 0 {
// 		cmax = 1
// 	}

// 	return cmin, cmax
// }
