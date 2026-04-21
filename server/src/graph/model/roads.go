package model

const (
	RoadMotorway     uint8 = 1
	RoadTrunk        uint8 = 2
	RoadPrimary      uint8 = 3
	RoadSecondary    uint8 = 4
	RoadTertiary     uint8 = 5
	RoadResidential  uint8 = 6
	RoadService      uint8 = 7
	RoadUnclassified uint8 = 8
)

const (
	defaultMotorwaySpeedKmh     = float32(120)
	defaultTrunkSpeedKmh        = float32(100)
	defaultPrimarySpeedKmh      = float32(80)
	defaultSecondarySpeedKmh    = float32(60)
	defaultTertiarySpeedKmh     = float32(50)
	defaultResidentialSpeedKmh  = float32(50)
	defaultServiceSpeedKmh      = float32(20)
	defaultUnclassifiedSpeedKmh = float32(50)
)

const (
	FlagOneWay uint8 = 1 << 0
	FlagToll   uint8 = 1 << 1
)

var HighwayClass = map[string]uint8{
	"motorway":       RoadMotorway,
	"motorway_link":  RoadMotorway,
	"trunk":          RoadTrunk,
	"trunk_link":     RoadTrunk,
	"primary":        RoadPrimary,
	"primary_link":   RoadPrimary,
	"secondary":      RoadSecondary,
	"secondary_link": RoadSecondary,
	"tertiary":       RoadTertiary,
	"tertiary_link":  RoadTertiary,
	"residential":    RoadResidential,
	"unclassified":   RoadUnclassified,
	"service":        RoadService,
}

// DefaultSpeedKmh returns the fallback speed for a road class when maxspeed is
// missing from the source data.
func DefaultSpeedKmh(class uint8) float32 {
	switch class {
	case RoadMotorway:
		return defaultMotorwaySpeedKmh
	case RoadTrunk:
		return defaultTrunkSpeedKmh
	case RoadPrimary:
		return defaultPrimarySpeedKmh
	case RoadSecondary:
		return defaultSecondarySpeedKmh
	case RoadTertiary:
		return defaultTertiarySpeedKmh
	case RoadResidential:
		return defaultResidentialSpeedKmh
	case RoadService:
		return defaultServiceSpeedKmh
	default:
		return defaultUnclassifiedSpeedKmh
	}
}
