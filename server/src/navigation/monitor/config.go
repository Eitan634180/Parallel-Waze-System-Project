package monitor

const (
	optimizationJobBufferSize = 256
	propagationBatchChunkSize = 64

	offRouteDistM      = float32(50)
	offRouteSanityMaxM = float32(5_000)
	offRouteWindow     = 2
)
