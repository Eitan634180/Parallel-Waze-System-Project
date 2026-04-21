package builder

const (
	minGraphBuilderWorkers     = 1
	estimatedEdgesPerWayHint   = 4
	pendingEdgesPerSegmentHint = 2

	minInertialFlowQuartile  = 1
	partitionSeed            = 42
	inertialFlowQuartileDiv  = 4
	flowTerminalNodeCount    = 2
	flowSinkNodeOffset       = 1
	unitFlowCapacity         = 1
	boundaryNodeCapacityHint = 32768
)
