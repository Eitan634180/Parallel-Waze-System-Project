package graph

const (
	MinGraphBuilderWorkers     = 1
	EstimatedEdgesPerWayHint   = 4
	PendingEdgesPerSegmentHint = 2

	MinInertialFlowQuartile  = 1
	PartitionSeed            = 42
	InertialFlowQuartileDiv  = 4
	FlowTerminalNodeCount    = 2
	FlowSinkNodeOffset       = 1
	UnitFlowCapacity         = 1
	BoundaryNodeCapacityHint = 32768

	SnapGridSize   = 128
	FileBufferSize = 1 << 20
)
