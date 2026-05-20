package executor

import (
	"sort"

	"github.com/zgiai/zgi/api/internal/capabilities/chunking/quality"
	"github.com/zgiai/zgi/api/internal/contracts"
)

type partitionResult struct {
	Partition           Partition
	Units               []contracts.ChunkUnit
	SourceFilterMetrics quality.FilterMetrics
}

func stableMerge(results []partitionResult) []contracts.ChunkUnit {
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].Partition.Index < results[j].Partition.Index
	})

	units := make([]contracts.ChunkUnit, 0)
	for _, result := range results {
		local := append([]contracts.ChunkUnit(nil), result.Units...)
		sort.SliceStable(local, func(i, j int) bool {
			return local[i].Order < local[j].Order
		})
		units = append(units, local...)
	}
	for i := range units {
		units[i].Order = i + 1
	}
	return units
}

func isStableMergedOrder(results []partitionResult) bool {
	results = append([]partitionResult(nil), results...)
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].Partition.Index < results[j].Partition.Index
	})

	last := 0
	for _, result := range results {
		local := append([]contracts.ChunkUnit(nil), result.Units...)
		sort.SliceStable(local, func(i, j int) bool {
			return local[i].Order < local[j].Order
		})
		for _, unit := range local {
			if unit.Order < last {
				return false
			}
			last = unit.Order
		}
	}
	return true
}

func isStableOrder(units []contracts.ChunkUnit) bool {
	for i := 1; i < len(units); i++ {
		if units[i].Order < units[i-1].Order {
			return false
		}
	}
	return true
}
